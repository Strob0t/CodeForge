"""Aider Polyglot benchmark provider.

Multi-language code editing benchmark derived from the Aider project.
Tasks span Python, JavaScript, Go, Rust, Java, C#, and more.
Each task provides a code file, an edit instruction, and a test suite.

Source: https://github.com/Aider-AI/aider (benchmark suite)
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "aider-ai/polyglot-benchmark"
_FILENAME = "aider_polyglot.jsonl"

# Language to test command mapping
_LANG_TEST_COMMANDS: dict[str, str] = {
    "python": "python -m pytest test_{name}.py -x",
    "javascript": "node --test test_{name}.js",
    "typescript": "npx ts-node test_{name}.ts",
    "go": "go test -run {name} -v",
    "rust": "cargo test {name}",
    "java": "javac {name}.java && java -cp . {name}Test",
    "csharp": "dotnet test",
    "cpp": "g++ -o test_{name} test_{name}.cpp && ./test_{name}",
    "ruby": "ruby test_{name}.rb",
    "php": "php test_{name}.php",
}


class AiderPolyglotProvider:
    """Multi-language edit benchmark from Aider project."""

    def __init__(
        self,
        cache_dir: str = "",
        language: str = "",
        tasks: list[dict] | None = None,
        config: dict | None = None,
    ) -> None:
        self._cache_dir = cache_dir
        self._language_filter = language.lower()
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "aider_polyglot"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(
            functional_tests=True,
            llm_judge=True,
            swe_bench_style=True,
        )

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        if self._language_filter:
            raw = [t for t in raw if t.get("language", "").lower() == self._language_filter]
        return [self._convert_task(t) for t in raw]

    async def task_count(self) -> int:
        tasks = await self.load_tasks()
        return len(tasks)

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="aider_polyglot",
            filename=_FILENAME,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        task_id = raw.get("task_id", raw.get("id", ""))
        task_name = raw.get("name", raw.get("task_name", str(task_id)))
        language = raw.get("language", "python").lower()
        instruction = raw.get("instruction", raw.get("prompt", ""))
        initial_code = raw.get("initial_code", raw.get("code", ""))
        test_code = raw.get("test_code", raw.get("test", ""))
        expected_code = raw.get("expected_code", raw.get("solution", ""))
        filename = raw.get("filename", f"solution.{_lang_ext(language)}")
        test_filename = raw.get("test_filename", f"test_{filename}")

        # Build initial_files for the agent workspace
        initial_files: dict[str, str] = {}
        if initial_code:
            initial_files[filename] = initial_code
        if test_code:
            initial_files[test_filename] = test_code

        # Determine test command from language
        test_cmd = raw.get("test_command", "")
        if not test_cmd:
            template = _LANG_TEST_COMMANDS.get(language, "")
            test_cmd = template.format(name=task_name) if template else ""

        full_instruction = f"Edit the file `{filename}` according to the following instruction.\n\n{instruction}"

        difficulty = raw.get("difficulty", "medium")
        if difficulty not in ("easy", "medium", "hard"):
            difficulty = "medium"

        return TaskSpec(
            id=f"aider_{task_id}",
            name=f"Aider_{task_name}"[:60],
            input=full_instruction,
            expected_output=expected_code,
            initial_files=initial_files,
            test_command=test_cmd,
            difficulty=difficulty,
            metadata={
                "language": language,
                "filename": filename,
                "test_filename": test_filename,
                "eval_method": "functional_test",
            },
        )


def _lang_ext(language: str) -> str:
    """Map language name to file extension."""
    extensions: dict[str, str] = {
        "python": "py",
        "javascript": "js",
        "typescript": "ts",
        "go": "go",
        "rust": "rs",
        "java": "java",
        "csharp": "cs",
        "cpp": "cpp",
        "ruby": "rb",
        "php": "php",
    }
    return extensions.get(language, "txt")


register_provider("aider_polyglot", AiderPolyglotProvider)
