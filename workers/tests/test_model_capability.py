"""Tool capability of a model: operator override, LiteLLM metadata, then the name (KI-125).

Before, the capability came from the model name only (a check through the
litellm library could never run: the worker does not install it), so local
models without "qwen3", "instruct" or "coder" in their names got no tools,
and a run without a model in its config was classified before a model was
chosen, as pure completion.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock

import httpx
import pytest

from codeforge.config import get_settings, load_yaml_config
from codeforge.executor import AgentExecutor
from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ModelMetadata, RoutingResult
from codeforge.loop_config import ModelCapability, resolve_model_capability
from codeforge.models import ConversationRunStartMessage, TaskMessage, TerminationConfig
from codeforge.runtime import RuntimeClient
from codeforge.tools import build_default_registry
from codeforge.tools.capability import CapabilityLevel, classify_model
from tests.jetstream_fakes import RecordingJetStream

if TYPE_CHECKING:
    from pathlib import Path

LOCAL = "ollama/llama3.1:8b"  # supports tools, but the name patterns say pure completion


def _row(name: str, *, fc: bool | None, window: int | None = None, litellm_model: str = "") -> dict[str, object]:
    return {
        "model_name": name,
        "litellm_params": {"model": litellm_model or name},
        "model_info": {"supports_function_calling": fc, "max_input_tokens": window},
    }


class MetadataLLM(LiteLLMClient):
    """A LiteLLMClient whose proxy serves /model/info from *rows*; every completion answers "done"."""

    def __init__(self, rows: list[dict[str, object]], status: int = 200) -> None:
        super().__init__(base_url="http://litellm.test", api_key="k")
        self.info_requests = 0
        self.calls: list[dict[str, object]] = []

        def handler(request: httpx.Request) -> httpx.Response:
            if request.url.path == "/model/info":
                self.info_requests += 1
                return httpx.Response(status, json={"data": rows})
            return httpx.Response(404)

        self._client = httpx.AsyncClient(base_url="http://litellm.test", transport=httpx.MockTransport(handler))

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:  # type: ignore[override]
        self.calls.append(kwargs)
        return ChatCompletionResponse(
            content="done", tool_calls=[], finish_reason="stop", tokens_in=1, tokens_out=1, model=str(kwargs["model"])
        )


@pytest.fixture
def overrides(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_MODEL_CAPABILITIES", "ollama/phi3*=pure_completion, ollama/*=api_with_tools")


# --- classify_model: override > metadata > name ---


@pytest.mark.parametrize(
    ("model", "fc", "expected"),
    [
        pytest.param(LOCAL, None, CapabilityLevel.PURE_COMPLETION, id="name-only-local"),
        pytest.param(LOCAL, True, CapabilityLevel.API_WITH_TOOLS, id="metadata-true-local"),
        pytest.param("openai/gpt-4o", True, CapabilityLevel.FULL, id="metadata-true-strong"),
        pytest.param("openai/gpt-4o", False, CapabilityLevel.PURE_COMPLETION, id="metadata-false"),
        pytest.param("ollama/qwen3:4b-instruct", None, CapabilityLevel.API_WITH_TOOLS, id="name-qwen3"),
        pytest.param("", True, CapabilityLevel.PURE_COMPLETION, id="empty-model"),
    ],
)
def test_metadata_before_name(model: str, fc: bool | None, expected: CapabilityLevel) -> None:
    assert classify_model(model, supports_function_calling=fc) == expected


@pytest.mark.usefixtures("overrides")
@pytest.mark.parametrize(
    ("model", "fc", "expected"),
    [
        pytest.param(LOCAL, None, CapabilityLevel.API_WITH_TOOLS, id="override-over-name"),
        pytest.param(LOCAL, False, CapabilityLevel.API_WITH_TOOLS, id="override-over-metadata-false"),
        pytest.param("ollama/phi3:mini", True, CapabilityLevel.PURE_COMPLETION, id="override-over-metadata-true"),
        pytest.param("ollama/phi3:mini", None, CapabilityLevel.PURE_COMPLETION, id="first-match-wins"),
        pytest.param("lm_studio/gemma-3", None, CapabilityLevel.PURE_COMPLETION, id="no-match-uses-name"),
        pytest.param("OLLAMA/LLAMA3", None, CapabilityLevel.PURE_COMPLETION, id="patterns-are-case-sensitive"),
    ],
)
def test_operator_override_wins(model: str, fc: bool | None, expected: CapabilityLevel) -> None:
    assert classify_model(model, supports_function_calling=fc) == expected


def test_override_from_yaml(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = tmp_path / "codeforge.yaml"
    config.write_text('litellm:\n  model_capabilities:\n    "ollama/*": api_with_tools\n    "*": full\n')
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    load_yaml_config.cache_clear()
    try:
        assert get_settings().model_capabilities == (("ollama/*", "api_with_tools"), ("*", "full"))
        assert classify_model(LOCAL) == CapabilityLevel.API_WITH_TOOLS
        assert classify_model("lm_studio/gemma-3") == CapabilityLevel.FULL
    finally:
        load_yaml_config.cache_clear()


def test_env_override_wins_over_yaml(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = tmp_path / "codeforge.yaml"
    config.write_text('litellm:\n  model_capabilities:\n    "ollama/*": full\n')
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    monkeypatch.setenv("CODEFORGE_MODEL_CAPABILITIES", "ollama/*=pure_completion")
    load_yaml_config.cache_clear()
    try:
        assert get_settings().model_capabilities == (("ollama/*", "pure_completion"),)
    finally:
        load_yaml_config.cache_clear()


@pytest.mark.parametrize(
    "yaml_value",
    [
        pytest.param('\n    - "ollama/*=api_with_tools"', id="list"),
        pytest.param(' "ollama/*=api_with_tools"', id="string"),
        pytest.param(" 3", id="number"),
    ],
)
@pytest.mark.parametrize("env", ["", "ollama/*=full"], ids=["yaml-only", "env-set"])
def test_wrongly_typed_yaml_override_stops_startup(
    yaml_value: str, env: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A list or a string was ignored without a word (KI-125 review): the operator's override did nothing."""
    config = tmp_path / "codeforge.yaml"
    config.write_text(f"litellm:\n  model_capabilities:{yaml_value}\n")
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    monkeypatch.setenv("CODEFORGE_MODEL_CAPABILITIES", env)
    load_yaml_config.cache_clear()
    try:
        with pytest.raises(ValueError, match=r"litellm\.model_capabilities"):
            get_settings()
    finally:
        load_yaml_config.cache_clear()


def test_empty_yaml_override_is_none(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = tmp_path / "codeforge.yaml"
    config.write_text("litellm:\n  model_capabilities:\n")
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    load_yaml_config.cache_clear()
    try:
        assert get_settings().model_capabilities == ()
    finally:
        load_yaml_config.cache_clear()


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        pytest.param("", (), id="unset"),
        pytest.param(" , ,", (), id="only-separators"),
        pytest.param(" a/* = full ,b=pure_completion", (("a/*", "full"), ("b", "pure_completion")), id="whitespace"),
    ],
)
def test_env_override_parsing(
    value: str, expected: tuple[tuple[str, str], ...], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("CODEFORGE_MODEL_CAPABILITIES", value)
    assert get_settings().model_capabilities == expected


@pytest.mark.parametrize(
    "value",
    [
        pytest.param("ollama/*", id="no-level"),
        pytest.param("ollama/*=tools", id="unknown-level"),
        pytest.param("=full", id="no-pattern"),
    ],
)
def test_invalid_override_is_refused(value: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_MODEL_CAPABILITIES", value)
    with pytest.raises(ValueError, match="CODEFORGE_MODEL_CAPABILITIES"):
        get_settings()


# --- LiteLLM /model/info ---


async def test_model_metadata_from_model_info() -> None:
    llm = MetadataLLM(
        [
            _row("lm_studio/*", fc=None, litellm_model="openai/*"),
            _row(LOCAL, fc=True, window=131072),
            _row("alias", fc=False, litellm_model="openai/gpt-4o-mini"),
        ]
    )

    assert await llm.model_metadata(LOCAL) == ModelMetadata(max_input_tokens=131072, supports_function_calling=True)
    assert await llm.model_metadata("openai/gpt-4o-mini") == ModelMetadata(supports_function_calling=False)
    assert await llm.model_metadata("lm_studio/qwen") == ModelMetadata()
    assert llm.info_requests == 1, "the table is cached"


@pytest.mark.parametrize("status", [401, 500])
async def test_model_metadata_unavailable(status: int) -> None:
    llm = MetadataLLM([_row(LOCAL, fc=True)], status=status)

    assert await llm.model_metadata(LOCAL) == ModelMetadata()
    assert await llm.model_metadata(LOCAL) == ModelMetadata()
    assert llm.info_requests == 2, "a failed lookup is not cached"


async def test_model_metadata_ignores_non_bool_and_non_int_values() -> None:
    row = {
        "model_name": LOCAL,
        "litellm_params": {},
        "model_info": {"supports_function_calling": "yes", "max_input_tokens": "big"},
    }
    llm = MetadataLLM([row])

    assert await llm.model_metadata(LOCAL) == ModelMetadata()


async def test_resolve_capability_uses_metadata_and_context_window() -> None:
    llm = MetadataLLM([_row(LOCAL, fc=True, window=20_000)])

    assert await resolve_model_capability(llm, LOCAL) == ModelCapability(CapabilityLevel.API_WITH_TOOLS, 17_000)


async def test_resolve_capability_without_a_litellm_client_uses_the_name() -> None:
    assert await resolve_model_capability(object(), LOCAL) == ModelCapability(CapabilityLevel.PURE_COMPLETION, 16_000)


# --- both loop paths ---


class _AllowAll(RecordingJetStream):
    async def publish(self, subject: str, payload: bytes = b"", **kwargs: object) -> object:
        ack = await super().publish(subject, payload, **kwargs)  # type: ignore[arg-type]
        if subject == "runs.toolcall.request":
            request = json.loads(payload)
            response = {"run_id": request["run_id"], "call_id": request["call_id"], "decision": "allow"}
            listening = [s for s in self.subscriptions if s.subject == "runs.toolcall.response" and not s.unsubscribed]
            listening[-1].deliver(json.dumps(response).encode())
        return ack


def _runtime(js: _AllowAll) -> RuntimeClient:
    return RuntimeClient(
        js=js,  # type: ignore[arg-type]
        notifications=js,  # type: ignore[arg-type]
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(max_steps=5, max_cost=5.0),
        approval_timeout_seconds=1,
    )


def _offered(llm: MetadataLLM) -> set[str]:
    return {t["function"]["name"] for t in llm.calls[0].get("tools") or []}  # type: ignore[index,union-attr]


@pytest.fixture
def no_router(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_hybrid_router", AsyncMock(return_value=None))
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=[]))


async def _run(llm: MetadataLLM, workspace: Path, config: dict[str, object]) -> None:
    task = TaskMessage(
        id="task-1",
        project_id="proj-1",
        title="Fix",
        prompt="Fix the bug in app.py",
        workspace_path=str(workspace),
        config=config,
    )
    await AgentExecutor(llm=llm).execute_with_runtime(task, _runtime(_AllowAll()))


@pytest.mark.usefixtures("no_router")
async def test_run_without_a_model_gets_the_default_models_tools(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("CODEFORGE_DEFAULT_MODEL", LOCAL)
    llm = MetadataLLM([_row(LOCAL, fc=True)])

    await _run(llm, tmp_path, {})

    assert llm.calls[0]["model"] == LOCAL
    assert {"read_file", "edit_file"} <= _offered(llm)


@pytest.mark.usefixtures("no_router", "overrides")
async def test_run_with_an_operator_marked_model_gets_tools(tmp_path: Path) -> None:
    llm = MetadataLLM([])

    await _run(llm, tmp_path, {"model": LOCAL})

    assert {"read_file", "edit_file"} <= _offered(llm)


async def test_conversation_loop_uses_the_metadata(tmp_path: Path) -> None:
    from codeforge.consumer import TaskConsumer

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://litellm.test")
    llm = MetadataLLM([_row(LOCAL, fc=True)])
    worker._llm = llm
    run_msg = ConversationRunStartMessage(
        run_id="run-1",
        conversation_id="c1",
        project_id="proj-1",
        messages=[],
        system_prompt="s",
        model=LOCAL,
        workspace_path=str(tmp_path),
        agentic=True,
    )

    await worker._execute_litellm_loop(
        run_msg,
        [{"role": "user", "content": "Fix the bug in app.py"}],
        LOCAL,
        RoutingResult(),
        _runtime(_AllowAll()),
        build_default_registry(),
        [],
    )

    assert {"read_file", "edit_file"} <= _offered(llm)


def _uses_the_text_protocol(call: dict[str, object]) -> bool:
    messages = call["messages"]
    system = str(messages[0]["content"]) if messages else ""  # type: ignore[index]
    fmt = call.get("response_format")
    return (
        call.get("tools") is None
        and isinstance(fmt, dict)
        and fmt.get("type") == "json_schema"
        and "## Tools\nYou work by calling tools." in system
    )


@pytest.mark.usefixtures("no_router")
async def test_model_without_function_calling_uses_the_text_protocol_on_both_paths(tmp_path: Path) -> None:
    """S9-C: a pure-completion model calls tools through the text protocol, in runs and conversations."""
    from codeforge.consumer import TaskConsumer

    run_llm = MetadataLLM([_row(LOCAL, fc=False)])
    await _run(run_llm, tmp_path, {"model": LOCAL})

    conversation_llm = MetadataLLM([_row(LOCAL, fc=False)])
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://litellm.test")
    worker._llm = conversation_llm
    run_msg = ConversationRunStartMessage(
        run_id="run-1",
        conversation_id="c1",
        project_id="proj-1",
        messages=[],
        system_prompt="s",
        model=LOCAL,
        workspace_path=str(tmp_path),
        agentic=True,
    )
    await worker._execute_litellm_loop(
        run_msg,
        [{"role": "system", "content": "s"}, {"role": "user", "content": "Fix the bug in app.py"}],
        LOCAL,
        RoutingResult(),
        _runtime(_AllowAll()),
        build_default_registry(),
        [],
    )

    assert _uses_the_text_protocol(run_llm.calls[0])
    assert _uses_the_text_protocol(conversation_llm.calls[0])
