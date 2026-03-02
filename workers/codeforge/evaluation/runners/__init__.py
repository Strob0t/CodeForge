"""Benchmark runners — dispatch tasks to LLM and collect results."""

from codeforge.evaluation.runners.simple import RunResult, SimpleBenchmarkRunner
from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner

__all__ = ["RunResult", "SimpleBenchmarkRunner", "ToolUseBenchmarkRunner"]
