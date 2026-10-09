"""Benchmark evaluation harness — DeepEval metrics, dataset management, and GEMMAS collaboration metrics."""

from codeforge.evaluation._deepeval_env import disable_deepeval_phone_home

# deepeval reads its telemetry settings when it is imported; every module that
# imports it lives in this package, so this runs first.
disable_deepeval_phone_home()
