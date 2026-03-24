"""Benchmark runner dispatch: simple, tool_use, agent, and multi-rollout wrappers."""

from __future__ import annotations

import tempfile

import structlog

from codeforge.config import get_settings

logger = structlog.get_logger()


def _dataset_to_task_specs(dataset_path: str) -> list:
    """Load dataset YAML and convert BenchmarkTasks to TaskSpec objects."""
    from codeforge.evaluation.datasets import load_dataset
    from codeforge.evaluation.providers.base import TaskSpec, ToolCall

    dataset = load_dataset(dataset_path)
    return [
        TaskSpec(
            id=t.id,
            name=t.name,
            input=t.input,
            expected_output=t.expected_output,
            expected_tools=[ToolCall(name=tc.get("name", ""), args=tc.get("args", "")) for tc in t.expected_tools],
            context=t.context,
            difficulty=t.difficulty,
        )
        for t in dataset.tasks
    ]


def _resolve_default_dataset(provider_name: str) -> str:
    """Map built-in provider names to their default dataset YAML paths."""
    from pathlib import Path

    settings = get_settings()
    datasets_dir = settings.benchmark_datasets_dir
    mapping = {
        "codeforge_simple": "basic-coding.yaml",
        "codeforge_tool_use": "tool-use-basic.yaml",
        "codeforge_agent": "agent-coding.yaml",
    }
    filename = mapping.get(provider_name, "")
    if not filename:
        return ""
    candidate = Path(datasets_dir) / filename
    if candidate.exists():
        return str(candidate)
    workspace = Path(settings.workspace)
    absolute = workspace / datasets_dir / filename
    return str(absolute) if absolute.exists() else ""


async def load_tasks_for_run(req: object) -> list:
    """Load tasks from provider registry or legacy YAML dataset."""
    from codeforge.evaluation.providers import get_provider
    from codeforge.evaluation.task_filter import apply_task_filters

    if req.provider_name:
        provider_cls = get_provider(req.provider_name)
        try:
            provider = provider_cls(config=req.provider_config)
        except TypeError:
            dataset_path = req.dataset_path
            if not dataset_path:
                dataset_path = _resolve_default_dataset(req.provider_name)
            provider = provider_cls(dataset_path=dataset_path)
        tasks = await provider.load_tasks()
        return apply_task_filters(tasks, req.provider_config)

    return _dataset_to_task_specs(req.dataset_path)


class BenchmarkRuntime:
    """Lightweight runtime stub for benchmark runs (no NATS dependency)."""

    def __init__(self, run_id: str = "benchmark") -> None:
        self.run_id = run_id
        self.project_id = ""
        self.is_cancelled = False

    async def send_output(self, _text: str) -> None:
        pass

    async def request_tool_call(self, **_kwargs: object) -> object:
        from codeforge.models import ToolCallDecision

        return ToolCallDecision(call_id="bench", decision="allow")

    async def report_tool_result(self, **_kwargs: object) -> None:
        pass

    async def publish_trajectory_event(self, **_kwargs: object) -> None:
        pass


async def run_simple_benchmark(
    req: object,
    llm: object,
    pipeline: object,
    on_start: object = None,
    on_complete: object = None,
    hybrid_pipeline: object = None,
) -> list:
    """Run a simple prompt -> LLM -> compare benchmark."""
    from codeforge.evaluation.runners.simple import SimpleBenchmarkRunner

    runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await load_tasks_for_run(req)
    return await run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete, hybrid_pipeline)


async def run_tool_use_benchmark(
    req: object,
    llm: object,
    pipeline: object,
    on_start: object = None,
    on_complete: object = None,
    hybrid_pipeline: object = None,
) -> list:
    """Run a tool-use benchmark with tools in task metadata."""
    from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner

    runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model=req.model)
    tasks = await load_tasks_for_run(req)
    return await run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete, hybrid_pipeline)


async def run_agent_benchmark(
    req: object,
    llm: object,
    pipeline: object,
    on_start: object = None,
    on_complete: object = None,
    hybrid_pipeline: object = None,
) -> list:
    """Run an agent benchmark using the full agent loop."""
    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.runners.agent import AgentBenchmarkRunner
    from codeforge.tools import build_default_registry

    if req.provider_name:
        tasks = await load_tasks_for_run(req)
    else:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        provider = CodeForgeAgentProvider(dataset_path=req.dataset_path)
        tasks = await provider.load_tasks()

    config = LoopConfig(
        model=req.model,
        max_cost=req.provider_config.get("max_cost", 1.0) if req.provider_config else 1.0,
    )
    registry = build_default_registry()
    runtime = BenchmarkRuntime(run_id=req.run_id)
    executor = AgentLoopExecutor(
        llm=llm,
        tool_registry=registry,
        runtime=runtime,
        workspace_path=tempfile.gettempdir(),
    )
    runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline, loop_config=config)
    return await run_with_optional_rollout(runner, tasks, req, pipeline, on_start, on_complete, hybrid_pipeline)


async def run_with_optional_rollout(
    runner: object,
    tasks: list,
    req: object,
    pipeline: object,
    on_start: object = None,
    on_complete: object = None,
    hybrid_pipeline: object = None,
) -> list:
    """Wrap runner in MultiRolloutRunner when rollout_count > 1."""
    from codeforge.consumer.benchmark_gemmas import convert_result, convert_rollout_outcome
    from codeforge.models import BenchmarkRunRequest

    if isinstance(req, BenchmarkRunRequest) and req.rollout_count > 1:
        from codeforge.evaluation.runners.multi_rollout import MultiRolloutRunner

        multi_runner = MultiRolloutRunner(
            inner_runner=runner,
            hybrid_pipeline=hybrid_pipeline,
            rollout_count=req.rollout_count,
            strategy=req.rollout_strategy,
        )
        results: list = []
        total = len(tasks)
        for i, task in enumerate(tasks):
            if on_start is not None:
                await on_start(task, i, total)
            outcomes = await multi_runner.run_task(task)
            converted = [convert_rollout_outcome(task, outcome, req.rollout_count) for outcome in outcomes]
            results.extend(converted)
            if on_complete is not None and converted:
                await on_complete(task, converted[0], i, total)
        return results

    run_results = await runner.run_tasks(tasks, on_task_start=on_start, on_task_complete=on_complete)
    return [convert_result(r) for r in run_results]
