"""Evaluator plugins for the benchmark evaluation pipeline.

Each evaluator implements the Evaluator protocol and produces an EvalDimension
score for a given task and execution result.

Available evaluators:
- FunctionalTestEvaluator: Runs shell test commands
- LLMJudgeEvaluator: Uses LLM to score outputs
- SPARCEvaluator: SPARC methodology scoring
- FilesystemStateEvaluator: Compares expected vs actual filesystem state
- TrajectoryVerifier: Verifies agent execution trajectories
- LogprobVerifier: Logprob-based verification
"""
