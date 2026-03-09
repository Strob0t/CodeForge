# Design: External Benchmark Providers, Full-Auto Routing & Prompt Optimization

> Date: 2026-03-09
> Status: Approved
> Scope: Benchmark system overhaul — external providers, auto-model routing, prompt feedback loop

---

## 1. Problem Statement

The benchmark system has 8 external providers implemented in Python (HumanEval, SWE-bench, MBPP, BigCodeBench, CRUXEval, LiveCodeBench, SPARCBench, Aider Polyglot) that are completely disconnected from the data flow. The frontend can only select local YAML datasets. There is no way to:

1. Run industry-standard benchmarks from the UI
2. Benchmark the routing system itself (full-auto model selection)
3. Track fallbacks, reachability, and rate limits during benchmark runs
4. Automatically improve agent prompts based on benchmark results
5. Adapt prompts per model family

---

## 2. Design Decision: Suite-Based Unified Architecture

**Chosen approach:** All benchmark sources (local YAML + external providers) are unified under the existing `benchmark_suites` entity. Suites become the single abstraction for selecting what to benchmark.

**Rationale:**
- Suite entity already exists with `provider_name`, `type`, `task_count`, `config` fields
- Follows the Provider Registry Pattern used throughout CodeForge (Git, PM, Spec providers)
- Python provider registry (`register_provider()` / `get_provider()`) maps directly to Suite.provider_name
- Leaderboard already filters by `suite_id` — cross-provider comparison works natively
- "There should be one — and preferably only one — obvious way to do it" (PEP 20)

**Alternatives rejected:**
- Unified Picker (A): Mixes file paths and provider names in one field — implicit, fragile
- Two Separate Pickers (B): Duplicates concepts, doesn't scale, breaks Registry Pattern

---

## 3. External Provider Integration

### 3.1 Available Providers

| Provider | Tasks | Type | Provider-Specific Settings |
|----------|------:|------|---------------------------|
| HumanEval | 164 | simple | `language` (python; future: HumanEval-X) |
| MBPP | 974 | simple | — |
| SWE-bench | 2294 | agent | `variant` (full/lite/verified), `repo_filter` |
| BigCodeBench | 1140 | simple | `subset` (complete/hard/instruct) |
| CRUXEval | 800 | simple | `task_type` (input_prediction/output_prediction) |
| LiveCodeBench | ~400+ | simple | `date_range`, `contest_filter` |
| SPARCBench | ~200 | agent | `category_filter` |
| Aider Polyglot | ~150 | agent | `language_filter` (python/js/ts/etc.) |

### 3.2 Provider Config Schema

Every provider receives a `config` dict (stored in Suite.config JSONB). Two levels:

**Universal settings (all providers):**

```yaml
max_tasks: 50          # 0 = all tasks
task_percentage: 100   # 0-100, alternative to max_tasks
difficulty_filter: []  # ["easy", "medium", "hard"] — empty = all
shuffle: true          # randomize task order before capping
seed: 42               # reproducibility
```

Resolution: `max_tasks` and `task_percentage` are mutually exclusive. The more restrictive wins. If both are set, effective count = min(max_tasks, ceil(total * percentage / 100)).

**Provider-specific settings** — only relevant keys are read by each provider. Unknown keys are ignored.

### 3.3 Suite Seeding

On Go server startup, seed default suites idempotently (skip if name + provider_name already exists):

**Local YAML datasets:**
```
basic-coding.yaml    -> Suite { name: "Basic Coding",    provider: "codeforge_simple",    type: simple, tasks: 5 }
agent-coding.yaml    -> Suite { name: "Agent Coding",    provider: "codeforge_agent",     type: agent,  tasks: 3 }
tool-use-basic.yaml  -> Suite { name: "Tool Use Basic",  provider: "codeforge_tool_use",  type: tool_use, tasks: 4 }
```

**External providers:**
```
Suite { name: "HumanEval",       provider: "humaneval",       type: simple, tasks: 164 }
Suite { name: "SWE-bench",       provider: "swebench",        type: agent,  tasks: 2294 }
Suite { name: "MBPP",            provider: "mbpp",            type: simple, tasks: 974 }
Suite { name: "BigCodeBench",    provider: "bigcodebench",    type: simple, tasks: 1140 }
Suite { name: "CRUXEval",        provider: "cruxeval",        type: simple, tasks: 800 }
Suite { name: "LiveCodeBench",   provider: "livecodebench",   type: simple, tasks: 400 }
Suite { name: "SPARCBench",      provider: "sparcbench",      type: agent,  tasks: 200 }
Suite { name: "Aider Polyglot",  provider: "aider_polyglot",  type: agent,  tasks: 150 }
```

Seeded suites are marked with `source: "builtin"` (new field or config flag) and cannot be deleted via UI.

---

## 4. Data Flow: Suite -> Provider -> Tasks -> Run

### 4.1 Current Flow (dataset-path based)

```
Frontend: dataset="basic-coding"
  -> Go: resolve to absolute file path
  -> NATS: dataset_path="/abs/path.yaml"
  -> Python: load_dataset(yaml) -> TaskSpecs
```

### 4.2 New Flow (suite-based)

```
1. Frontend: suite_id="abc-123", provider_config={max_tasks: 50, variant: "lite"}

2. Go StartRun():
   - Load Suite from DB by suite_id
   - Extract provider_name, merge Suite.config with request provider_config
   - Build NATS payload with provider_name + provider_config
   - Fallback: if dataset is set and suite_id is empty -> old path (backwards compat)

3. NATS payload (new fields):
   {
     run_id, tenant_id, model, metrics,
     provider_name: "humaneval",
     provider_config: { max_tasks: 50, shuffle: true, seed: 42 },
     dataset_path: "",  // empty for provider-based runs
     benchmark_type: "simple",
     suite_id: "abc-123",
     ...existing fields...
   }

4. Python Consumer (_handle_benchmark_run):
   if provider_name:
       provider_cls = get_provider(provider_name)
       provider = provider_cls(config=provider_config)
       tasks = await provider.load_tasks()
       tasks = _apply_task_filters(tasks, provider_config)
   else:
       tasks = _dataset_to_task_specs(dataset_path)  # legacy fallback

5. Runner + Evaluators proceed as before
```

### 4.3 Universal Task Filter Function

```python
def _apply_task_filters(tasks: list[TaskSpec], config: dict) -> list[TaskSpec]:
    # 1. Difficulty filter
    difficulties = config.get("difficulty_filter", [])
    if difficulties:
        tasks = [t for t in tasks if t.difficulty in difficulties]

    # 2. Shuffle
    if config.get("shuffle", True):
        import random
        rng = random.Random(config.get("seed", 42))
        rng.shuffle(tasks)

    # 3. Cap by max_tasks or task_percentage
    max_tasks = config.get("max_tasks", 0)
    percentage = config.get("task_percentage", 100)
    if percentage < 100:
        cap_by_pct = max(1, int(len(tasks) * percentage / 100))
        tasks = tasks[:cap_by_pct]
    if max_tasks > 0:
        tasks = tasks[:max_tasks]

    return tasks
```

### 4.4 NATS Payload Changes

**BenchmarkRunRequestPayload** — add fields:
```go
ProviderName   string          `json:"provider_name,omitempty"`
ProviderConfig json.RawMessage `json:"provider_config,omitempty"`
```

**Python BenchmarkRunRequest** — add fields:
```python
provider_name: str = ""
provider_config: dict[str, Any] = {}
```

Existing `dataset_path` field remains for backwards compatibility.

---

## 5. Full-Auto Model Routing in Benchmarks

### 5.1 Concept

Instead of selecting a specific model, the user selects `model: "auto"`. The existing HybridRouter (Phase 29) makes per-task model decisions. The benchmark tests the entire routing system as a real user would experience it.

### 5.2 Run-Level Tracking

When `model="auto"`, the Run stores `model: "auto"` in DB. Each Result gains new fields:

```go
// New fields on BenchmarkResult (and BenchmarkTaskResult NATS payload)
SelectedModel    string `json:"selected_model,omitempty"`     // which model the router chose
RoutingReason    string `json:"routing_reason,omitempty"`     // "complexity:high,ucb1:0.87"
FallbackChain    string `json:"fallback_chain,omitempty"`     // "anthropic->mistral" if fallback occurred
FallbackCount    int    `json:"fallback_count,omitempty"`     // number of fallback attempts
ProviderErrors   string `json:"provider_errors,omitempty"`    // "anthropic:402,groq:429"
```

### 5.3 Aggregated Routing Report

The Run summary gains a `routing_report` section:

```json
{
  "summary_scores": { "correctness": 0.87 },
  "routing_report": {
    "models_used": {
      "anthropic/claude-sonnet-4": {
        "task_count": 12,
        "task_percentage": 40.0,
        "avg_score": 0.92,
        "avg_cost_per_task": 0.015,
        "difficulty_distribution": { "easy": 2, "medium": 5, "hard": 5 }
      },
      "mistral/mistral-large-latest": {
        "task_count": 18,
        "task_percentage": 60.0,
        "avg_score": 0.84,
        "avg_cost_per_task": 0.003,
        "difficulty_distribution": { "easy": 10, "medium": 6, "hard": 2 }
      }
    },
    "fallback_events": 3,
    "fallback_details": [
      { "task_id": "task-7", "primary": "anthropic", "fallback_to": "mistral", "reason": "402 billing" },
      { "task_id": "task-15", "primary": "groq", "fallback_to": "mistral", "reason": "429 rate_limit" }
    ],
    "provider_availability": {
      "anthropic": { "reachable": true, "errors": 1, "error_types": ["billing"] },
      "mistral": { "reachable": true, "errors": 0 },
      "groq": { "reachable": true, "errors": 2, "error_types": ["rate_limit"] }
    },
    "system_score": 0.87,
    "system_cost": 0.24,
    "cost_vs_single_model": {
      "savings_vs_best": "-12%",
      "note": "Auto-routing saved 12% cost vs running all on claude-sonnet"
    }
  }
}
```

### 5.4 Implementation: Python Consumer Changes

In `_handle_benchmark_run`, when `model == "auto"`:
- Import HybridRouter
- Before each task: call `router.select_model(task_input, complexity_hint=task.difficulty)`
- Wrap LLM call in fallback logic: primary fails -> record error -> try next model
- Attach `selected_model`, `routing_reason`, `fallback_chain`, `provider_errors` to each result
- After all tasks: compute `routing_report` summary and include in BenchmarkRunResult

### 5.5 DB Schema Changes

**benchmark_results table** — new nullable columns:
```sql
selected_model TEXT DEFAULT '',
routing_reason TEXT DEFAULT '',
fallback_chain TEXT DEFAULT '',
fallback_count INTEGER DEFAULT 0,
provider_errors TEXT DEFAULT ''
```

**benchmark_runs table** — routing_report stored in existing `summary_scores` JSONB (or new `routing_report JSONB` column).

---

## 6. Prompt Optimization Feedback Loop

### 6.1 Research Basis

Hybrid approach inspired by three key frameworks:

- **SICA** (Self-Improving Coding Agent, ICLR 2025): Agent analyzes own failures, proposes code/prompt changes. 17% -> 53% on SWE-bench Verified.
- **SCOPE** (Prompt Evolution, Dec 2025): Dual-Stream — tactical fixes (specific errors) + strategic principles (long-term patterns). 14% -> 38% on HLE.
- **MIPROv2** (DSPy/Stanford): Bootstrap successful traces as few-shot examples. 24% -> 51% on HotPotQA.

### 6.2 Architecture: Three-Phase Optimization

```
Phase A: Analyze
  Input: Benchmark results (failures grouped by mode + model-family)
  Process: LLM-as-Critic analyzes failed tasks with full trajectory
  Output: PromptAnalysisReport

Phase B: Propose
  Input: PromptAnalysisReport + current mode prompts
  Process: Generate prompt diffs (tactical fixes + strategic principles)
  Output: PromptPatch[] (one per mode x model-family)

Phase C: Validate
  Input: PromptPatch[] applied to prompts
  Process: Re-run same benchmark suite with patched prompts
  Output: Score delta (improved / regressed / neutral)
```

### 6.3 PromptAnalysisReport Structure

```python
@dataclass
class PromptAnalysisReport:
    suite_id: str
    run_id: str
    mode: str                          # e.g., "coder"
    model_family: str                  # e.g., "meta-llama"
    total_tasks: int
    failed_tasks: int
    failure_rate: float

    tactical_fixes: list[TacticalFix]  # SCOPE-style: specific error -> specific fix
    strategic_principles: list[str]    # SCOPE-style: overarching patterns
    few_shot_candidates: list[str]     # MIPROv2-style: successful traces as examples

@dataclass
class TacticalFix:
    task_id: str
    failure_description: str           # what went wrong
    root_cause: str                    # why (from LLM analysis)
    proposed_addition: str             # text to add to prompt
    confidence: float                  # 0-1, LLM self-assessed
```

### 6.4 PromptPatch Structure

```python
@dataclass
class PromptPatch:
    mode: str                          # target mode (e.g., "coder")
    model_family: str                  # target family (e.g., "meta-llama")
    patch_type: str                    # "tactical" | "strategic" | "few_shot"
    action: str                        # "add" | "replace" | "remove"
    content: str                       # the actual prompt text to add/replace
    location: str                      # "model_adaptations" | "prompt_template" | "few_shots"
    rationale: str                     # why this change helps
    source_task_ids: list[str]         # which failures motivated this
```

### 6.5 Approval Modes

**Stufe 2 (default): User Review**
- Prompt patches displayed in UI as diffs
- User can Accept / Reject / Edit each patch
- Accepted patches are written to mode YAML
- Re-run benchmark button to validate improvement

**Stufe 3 (opt-in): Auto-Iterate**
- Enabled via config flag: `prompt_optimization.auto_iterate: true`
- Loop: Benchmark -> Analyze -> Patch -> Re-Benchmark -> Compare
- Convergence criteria: score delta < 1% OR max iterations (default: 5) OR budget limit
- All iterations tracked in DB for audit trail
- Rollback to best-performing iteration if final is worse

### 6.6 Integration Points

- **Experience Pool** (`@exp_cache`): Successful traces from benchmarks feed into few-shot candidates (MIPROv2 Bootstrap pattern)
- **Trajectory System**: Full execution traces used as input for LLM-as-Critic analysis
- **Modes System**: Patches target mode YAML files (model_adaptations section)
- **Cost Tracking**: Optimization loop cost tracked separately from benchmark cost

---

## 7. Per-Model Prompt Adaptation (Prompt Layer Architecture)

### 7.1 Three-Layer Prompt Stack

```
Layer 3: Model-Family Adaptation     <- NEW (per model family)
Layer 2: Mode Instructions           <- EXISTS (coder.tmpl, reviewer.tmpl)
Layer 1: Base System Prompt          <- EXISTS (project context, tools, safety)
```

Final prompt = Layer 1 + Layer 2 + Layer 3 + Skills + Goals + Optimized Patches

### 7.2 Model Family Classification

```
Family         | Match Pattern           | Prompt Style
---------------|-------------------------|----------------------------------------
anthropic      | anthropic/*, claude-*   | Concise constraints, explicit "don't" rules
openai         | openai/*, gpt-*, o1-*   | Structured formats, JSON hints
mistral        | mistral/*, mixtral-*    | Direct instructions, shorter prompts
meta-llama     | meta-llama/*, llama-*   | Few-shot examples, step-by-step, simple language
google         | google/*, gemini-*      | Similar to openai, good structured output
local          | ollama/*, lm-studio/*   | Maximum explicitness, few-shot mandatory, short tool descriptions
```

Family is derived from the model name prefix at runtime. The `routing/` package already parses provider prefixes.

### 7.3 Mode YAML Extension

```yaml
# modes/coder.yaml
name: coder
prompt_template: coder.tmpl
model_adaptations:
  anthropic: |
    Be concise. Never explain what you are about to do, just do it.
    Do NOT add comments to obvious code.
  openai: |
    When outputting code, always use markdown fenced code blocks with language tags.
  meta-llama: |
    Always output the COMPLETE file content. Never use "..." or "rest unchanged".
    Follow this exact sequence:
    1. Read the relevant file
    2. Plan changes in 2-3 bullet points
    3. Write the complete updated file
  local: |
    You have these tools: {{.ToolList}}
    Use tools one at a time. Wait for each result before proceeding.
    Keep responses under 500 tokens.
```

### 7.4 Prompt Assembly (Go)

```go
func buildSystemPrompt(base, modeInstructions, modelFamily string, adaptations map[string]string) string {
    prompt := base + "\n\n" + modeInstructions
    if adaptation, ok := adaptations[modelFamily]; ok {
        prompt += "\n\n" + adaptation
    }
    return prompt
}
```

Model family is extracted via prefix matching in a utility function:
```go
func ModelFamily(modelName string) string // "anthropic/claude-sonnet-4" -> "anthropic"
```

### 7.5 Prompt Optimizer Writes to model_adaptations

When the optimization loop generates patches for a specific model family:
- Patch targets `modes/{mode}.yaml -> model_adaptations.{family}`
- Existing adaptations are preserved; new text is appended or section is replaced
- Version history tracked (git commits or DB audit trail)

---

## 8. Frontend Changes

### 8.1 Run Form (BenchmarkPage.tsx)

**Suite Dropdown** replaces Dataset dropdown:
- Grouped `<optgroup>`: "Local" and "External"
- Shows: name, type badge, task count
- On selection: auto-fills benchmark_type from Suite.type (overridable)

**Model Field** gains "Auto" option:
- ModelCombobox gets a special "auto (intelligent routing)" entry
- When selected, sends `model: "auto"` to API

**Task Settings** (new collapsible section):
- Radio: All / Limit / Percentage
- Difficulty checkboxes
- Shuffle + Seed
- Provider-specific settings rendered dynamically from `PROVIDER_SETTINGS` map

**Estimated Cost Hint:**
- Below task settings: `~50 tasks x ~$0.01/task = $0.50`

### 8.2 Run Detail (BenchmarkRunDetail.tsx)

**Routing Report** (shown when model="auto"):
- Model distribution pie chart or bar
- Per-model stats table (tasks, avg score, avg cost, difficulty breakdown)
- Fallback events timeline
- Provider availability status indicators

### 8.3 Prompt Optimization UI (new component)

**PromptOptimizationPanel** (accessible from Run Detail when completed):
- "Analyze & Suggest Improvements" button
- Shows PromptAnalysisReport: failure breakdown by mode + model-family
- Tactical fixes listed with confidence scores
- Strategic principles as bullet points
- Prompt diffs with syntax highlighting (green=add, red=remove)
- Accept / Reject / Edit per patch
- "Re-Run Benchmark" button to validate
- Score comparison: before vs after

### 8.4 Suite Management (SuiteManagement.tsx)

- `provider_name` becomes dropdown (known providers)
- `type` auto-filled from provider
- Config editor: structured fields for known settings, JSON fallback for custom
- Seeded suites show lock icon, not deletable

### 8.5 Type Changes (types.ts)

```typescript
interface CreateBenchmarkRunRequest {
  suite_id: string;                    // replaces dataset
  model: string;                       // "auto" for full-auto routing
  metrics: string[];
  benchmark_type?: BenchmarkType;
  exec_mode?: BenchmarkExecMode;
  provider_config?: ProviderConfig;    // NEW: task filtering + provider settings
}

interface ProviderConfig {
  max_tasks?: number;
  task_percentage?: number;
  difficulty_filter?: string[];
  shuffle?: boolean;
  seed?: number;
  [key: string]: unknown;              // provider-specific keys
}

interface BenchmarkResult {
  // ...existing fields...
  selected_model?: string;             // NEW: which model was chosen (auto mode)
  routing_reason?: string;             // NEW
  fallback_chain?: string;             // NEW
  fallback_count?: number;             // NEW
  provider_errors?: string;            // NEW
}

interface RoutingReport {
  models_used: Record<string, ModelUsageStats>;
  fallback_events: number;
  fallback_details: FallbackEvent[];
  provider_availability: Record<string, ProviderStatus>;
  system_score: number;
  system_cost: number;
}
```

### 8.6 New i18n Keys

```
benchmark.suite              -> "Benchmark Suite"
benchmark.suiteLocal         -> "Local"
benchmark.suiteExternal      -> "External"
benchmark.taskSettings       -> "Task Settings"
benchmark.allTasks           -> "All ({count})"
benchmark.limitTasks         -> "Limit to"
benchmark.percentTasks       -> "Percentage"
benchmark.difficulty         -> "Difficulty"
benchmark.shuffle            -> "Shuffle"
benchmark.seed               -> "Random Seed"
benchmark.estimatedCost      -> "Estimated: ~{tasks} tasks x ~${cost}/task = ${total}"
benchmark.providerSettings   -> "Provider Settings"
benchmark.modelAuto          -> "Auto (intelligent routing)"
benchmark.routingReport      -> "Routing Report"
benchmark.fallbackEvents     -> "Fallback Events"
benchmark.providerStatus     -> "Provider Availability"
benchmark.promptOptimization -> "Prompt Optimization"
benchmark.analyzePrompts     -> "Analyze & Suggest Improvements"
benchmark.promptDiff         -> "Suggested Changes"
benchmark.acceptPatch        -> "Accept"
benchmark.rejectPatch        -> "Reject"
benchmark.rerunBenchmark     -> "Re-Run to Validate"
benchmark.scoreImproved      -> "Score improved: {before} -> {after} (+{delta})"
benchmark.scoreRegressed     -> "Score regressed: {before} -> {after} ({delta})"
```

---

## 9. Migration & Backwards Compatibility

### 9.1 DB Migration

New migration adds columns to `benchmark_results`:
```sql
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS selected_model TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS routing_reason TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_chain TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_count INTEGER DEFAULT 0;
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS provider_errors TEXT DEFAULT '';
```

No schema changes to `benchmark_suites` or `benchmark_runs` — existing `config` JSONB and `summary_scores` JSONB absorb new data.

### 9.2 Backwards Compatibility

- `CreateBenchmarkRunRequest.dataset` remains as optional field
- If `dataset` is set and `suite_id` is empty -> legacy dataset-path flow
- If `suite_id` is set -> new provider-based flow
- Existing runs with `dataset` field continue to display correctly
- Python consumer checks `provider_name` first, falls back to `dataset_path`

### 9.3 Mode YAML Compatibility

- `model_adaptations` is a new optional key in mode YAML
- Existing modes without it work unchanged (no Layer 3 prompt)
- Prompt optimizer only writes `model_adaptations` when it has data

---

## 10. File Locations (planned)

### Go Changes
- `internal/domain/benchmark/benchmark.go` — new result fields, RoutingReport types
- `internal/service/benchmark.go` — suite-based StartRun, suite seeding on startup
- `internal/adapter/http/handlers_benchmark.go` — prompt optimization endpoints
- `internal/port/messagequeue/schemas.go` — ProviderName, ProviderConfig fields
- `internal/adapter/postgres/migrations/068_benchmark_routing_fields.sql`

### Python Changes
- `workers/codeforge/consumer/_benchmark.py` — provider-based task loading, auto routing
- `workers/codeforge/models.py` — new NATS payload fields
- `workers/codeforge/evaluation/providers/*.py` — config support in constructors
- `workers/codeforge/evaluation/task_filter.py` — universal filter function (NEW)
- `workers/codeforge/evaluation/prompt_optimizer.py` — analyzer + patcher (NEW)

### Frontend Changes
- `frontend/src/features/benchmarks/BenchmarkPage.tsx` — suite dropdown, task settings
- `frontend/src/features/benchmarks/RoutingReport.tsx` — NEW component
- `frontend/src/features/benchmarks/PromptOptimizationPanel.tsx` — NEW component
- `frontend/src/features/benchmarks/TaskSettings.tsx` — NEW component
- `frontend/src/features/benchmarks/SuiteManagement.tsx` — provider dropdown, config
- `frontend/src/api/types.ts` — new interfaces
- `frontend/src/i18n/en.ts` + `de.ts` — new keys

### Mode System Changes
- `modes/*.yaml` — add `model_adaptations` section (initially empty, populated by optimizer)

---

## 11. Research References

- [SICA: A Self-Improving Coding Agent](https://arxiv.org/abs/2504.15228) — ICLR 2025 Workshop
- [SCOPE: Prompt Evolution](https://arxiv.org/abs/2512.15374) — Dec 2025
- [DSPy MIPROv2](https://dspy.ai/api/optimizers/MIPROv2/) — Stanford
- [TextGrad](https://textgrad.com/) — Stanford, Nature 2024
- [EvoMAC](https://proceedings.iclr.cc/paper_files/paper/2025/file/39af4f2f9399122a14ccf95e2d2e7122-Paper-Conference.pdf) — ICLR 2025
- [GEPA — Databricks](https://www.databricks.com/blog/building-state-art-enterprise-agents-90x-cheaper-automated-prompt-optimization)
- [CPO: Causal Prompt Optimization](https://papers.ssrn.com/sol3/Delivery.cfm/6073587.pdf?abstractid=6073587&mirid=1) — Jan 2026
- [Awesome Self-Evolving Agents Survey](https://github.com/EvoAgentX/Awesome-Self-Evolving-Agents)
