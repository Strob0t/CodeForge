# Small coder model smoke test (2026-10-06)

Which small local model should drive the live E2E sessions and the autonomous-goal benchmark instead of
`qwen3:4b-instruct`? The research ranked nine candidates that fit the 8 GB / 4 CPU core budget of the
benchmark host (Ollama 0.35.1, Emerald Rapids VM, no GPU); this smoke test measured the top candidate on that
host against the baseline. Owner decision (2026-10-06): **`qwen3.5:4b-q4_K_M`** replaces `qwen3:4b-instruct`
as the default of `scripts/live-e2e/` (`LIVE_MODEL`) and of the benchmark runs.

## Candidates (research, not measured here unless noted)

| # | Model (Ollama tag or HF GGUF) | Params | Q4 file | RAM at 32k context | Tool-use evidence | Coding evidence | Thinking |
|---|---|---|---|---|---|---|---|
| 1 | Qwen3.5-4B, `qwen3.5:4b-q4_K_M` | 4B dense hybrid | 3.3 GB | ~5 GB (KV 1.1 GB) | BFCL-V4 50.3, TAU2 79.9, IFEval 89.8 | LCB v6 55.8; SWE-V 39.4 in a read/write/edit/glob/bash loop | on by default, `reasoning_effort: none` turns it off |
| 2 | FrogNano-4B-2609 (Qwen3.5-4B + coding-agent RL), HF only | 4B | 2.8 GB | ~4.5 GB | none published | SWE-V 61.5 on its validation set; SWE-Pro 37.6 | trained with thinking on (8-32k tokens per turn) |
| 3 | MiniCPM5-2B, HF only | 2.5B dense | 1.6 GB | ~3.5 GB | BFCL v4 66.6 (vendor) | LCB v6 69.1, SWE-V 46.4 (vendor) | hybrid, `enable_thinking=false` |
| 4 | Granite 4.2 3B, `granite4.2:3b` | 3.7B dense | 2.2 GB | ~5.4 GB | BFCL v4 52.4; IFEval 93.7 | LCB v6 69.7; SWE-V 36.8 | on by default; Ollama toggle unverified |
| 5 | Qwen3.5-9B, HF Q3_K_M | 9B hybrid | 4.7 GB | ~6.4 GB | BFCL-V4 66.1 | SWE-V 53.2 (OpenHands) | as #1 |
| 6 | Granite 4.2 8B, `granite4.2:8b-q4_K_S` | 8B dense | 5.1 GB | ~8.5 GB (q8 KV) | BFCL v4 52.4 | SWE-V 47.7 | as #4 |
| 7 | Ornith-1.5-9B, HF Q3_K_M | 9B | 4.9 GB | ~6.7 GB | MCP-Atlas 54.2 | SWE-V 70.6 (vendor); an independent test failed | reasoning only |
| 8 | LFM2.5-8B-A1B, `lfm2.5:8b-a1b-q4_K_M` | 8.3B / 1.5B MoE | 5.2 GB | ~6.1 GB | BFCL v3 64.8 | vendor: not for heavy programming; SWE-V 0.4 | reasoning only; restrictive license |
| 9 | Nemotron 3 Nano 4B, `nemotron-3-nano:4b` | 4B Mamba-2 hybrid | 2.8 GB | ~3.9 GB | BFCL v3 61.1 | LCB 51.8; SWE-V 3.0 | toggle |
| base | `qwen3:4b-instruct` (2507) | 4B dense | 2.5 GB | ~7.9 GB (KV 4.8 GB) | BFCL v3 61.9; BFCL v4 35.7 | LCB 35.1; Aider-Polyglot 12.9 | none |

Benchmark numbers are the vendors' or leaderboards' (BFCL = Berkeley Function Calling Leaderboard, LCB =
LiveCodeBench, SWE-V = SWE-bench Verified, TB = Terminal-Bench); they were not reproduced here.

## What was measured

Scripts in the session scratchpad (`model-smoke/`, not in the repository): they call Ollama's native API with
`num_ctx` 16384 and the worker's real `TextToolProtocol` (prompt section, turn grammar, `wire_messages`,
`parse_tool_turn`) with canned tool results, so they test the text tool protocol (ADR-021), not native tool
calling.

- **A. Speed:** an 8k-token prefill, 64 generated tokens, then the same prompt again (prefix-cache reuse).
- **B. Protocol episodes:** six scripted tasks, once *constrained* (the JSON-schema grammar of
  `litellm.text_tool_grammar`) and once *free* (no grammar): (1) create and run `hello.py`; (2) find where
  `parse_tool_turn` is defined; (3) fix a failing pytest project without touching the tests; (4) `17*23`;
  (5) write `strings_util.py` with tests and run pytest; (6) read `config/settings.yaml` and report
  `max_workers`. A protocol error gets one repair message; a second one ends the episode.
- **C. Code micro-check:** write `extract_links` (Markdown links, ignoring images and code spans) with own
  tests, then run hidden tests.

The host was shared with other test runs during the measurement, so every speed figure is a lower bound.

## Results

| | `qwen3:4b-instruct` (baseline) | `qwen3.5:4b-q4_K_M` (thinking off) |
|---|---|---|
| Prefill, 8k tokens (4 threads, loaded host) | 22 tok/s | 20 tok/s |
| Generation | 1.8 tok/s (loaded host; 9 tok/s idle, earlier sessions) | 0.3 tok/s (loaded host; 7-9 tok/s expected idle) |
| Prefix-cache reuse (second prompt) | 2.5 s prefill (cache hit) | 39 s prefill (partial reuse only) |
| B constrained (grammar on) | **0/6** solved; every episode ended after one turn | **5/6** solved (ep 6 answered correctly but not in the final object's format) |
| B free (grammar off) | 5/6 (ep 4 failed) | 5/6 (ep 6, as above); two turns with minor format slips, repaired |
| C code micro-check | 1/5 own, 1/6 hidden tests pass (wrong regex group) | 0 (its output was cut at a string literal: syntax error) |

Key findings:

- **The JSON-schema grammar breaks the baseline.** Under the grammar the baseline emits the object keys in
  alphabetical order (`final` before `thought`) and ends every episode after one turn. Qwen3.5-4B follows the
  grammar and solves 5/6. Text tool protocol users with `qwen3:4b-instruct` must set
  `CODEFORGE_TEXT_TOOL_GRAMMAR=false` ([KI-225](../todo.md#known-issues)).
- **Thinking must be off.** Qwen3.5 thinks by default; with thinking on, a turn at CPU speed can take many
  minutes. The worker sends `reasoning_effort: "none"` for local models (see below).
- **Prompt-cache reuse is weaker** for the hybrid (DeltaNet) architecture in Ollama 0.35.1: the second
  prompt re-processed most of the prefix. Multi-turn agent runs are therefore slower per turn than with the
  baseline at the same tokens/s; the KV cache at 32k is 1.1 GB instead of 4.8 GB, so a 32k context fits.
- **Neither model passed the code micro-check**: at this size, correctness of non-trivial code still needs
  the test-and-fix loop (episode 3 and 5 show both models can run and fix tests).

## What changed

- `scripts/live-e2e/env.example.sh`: `LIVE_MODEL` defaults to `ollama/qwen3.5:4b-q4_K_M`.
- The worker sends `reasoning_effort: "none"` with every local model call (`workers/codeforge/loop_config.py`),
  configurable with `litellm.local_reasoning_effort` / `CODEFORGE_LOCAL_REASONING_EFFORT` (empty: send nothing).
- Not measured: Granite 4.2 3B and MiniCPM5-2B (candidates 3 and 4) on this host; the native tool-calling path
  of Qwen3.5 through LiteLLM; a full benchmark run with the new model ([autonomous-goal-benchmark.md](autonomous-goal-benchmark.md), run 2).
