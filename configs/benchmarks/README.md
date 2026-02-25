# Benchmark Datasets

This directory contains YAML benchmark datasets used by CodeForge's evaluation framework (Phase 20).

## YAML Schema

Each dataset file must follow this structure:

```yaml
name: "Dataset Name"
description: "What this dataset evaluates."

tasks:
  - id: "unique-task-id"
    name: "Human-readable task name"
    input: |
      The prompt sent to the LLM.
    expected_output: |
      The reference answer used for scoring.
    difficulty: "easy"          # easy | medium | hard (default: medium)
    expected_tools: []          # Optional: list of expected tool calls
    context: []                 # Optional: retrieval context for faithfulness
```

## Task Fields

| Field | Required | Type | Description |
|---|---|---|---|
| `id` | yes | string | Unique identifier within the dataset |
| `name` | yes | string | Human-readable task name |
| `input` | yes | string | Prompt sent to the LLM |
| `expected_output` | yes | string | Reference answer for scoring |
| `difficulty` | no | string | `easy`, `medium`, or `hard` (default: `medium`) |
| `expected_tools` | no | list | Expected tool calls `[{name, args}]` for tool_correctness metric |
| `context` | no | list[str] | Retrieval context passages for faithfulness metric |

## Available Metrics

| Metric | Description |
|---|---|
| `correctness` | G-Eval: factual match between actual and expected output |
| `tool_correctness` | G-Eval: correct tool calls in the right order |
| `faithfulness` | DeepEval: output faithful to retrieval context |
| `answer_relevancy` | DeepEval: output relevant to the input question |
| `contextual_precision` | DeepEval: precision of context retrieval |

## Adding Custom Datasets

1. Create a new `.yaml` file in this directory following the schema above.
2. The file is auto-discovered by the benchmark runner via the `benchmark.datasets_dir` config key.
3. Start a benchmark run via the API or the frontend dashboard.

Example minimal dataset:

```yaml
name: My Custom Benchmark
description: Tests for domain-specific code generation.

tasks:
  - id: custom-001
    name: Generate API endpoint
    input: "Write a Go HTTP handler that returns JSON."
    expected_output: |
      func handler(w http.ResponseWriter, r *http.Request) {
          w.Header().Set("Content-Type", "application/json")
          json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
      }
```

## Running Benchmarks

```bash
# Via API
curl -X POST http://localhost:8080/api/v1/benchmarks/runs \
  -H "Content-Type: application/json" \
  -d '{"dataset": "basic-coding", "model": "openai/gpt-4o", "metrics": ["correctness"]}'

# Via frontend
# Navigate to the Benchmarks page in the CodeForge UI (dev mode only).
```
