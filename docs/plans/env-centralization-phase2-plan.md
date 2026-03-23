# Environment Variable Centralization Phase 2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize all 46+ scattered `os.getenv()`/`os.environ.get()` calls in Python worker into `WorkerSettings`, fix 1 remaining Go `os.Getenv`, add 17 missing env vars to docs.

**Architecture:** Extend `WorkerSettings` in `workers/codeforge/config.py` with all missing vars using existing `_resolve_*` helpers. Add a cached `get_settings()` singleton. Replace scattered reads with `get_settings().field_name`. For Go, move 1 remaining `os.Getenv` into config loader.

**Tech Stack:** Python 3.12, Go 1.25

---

### Task 1: Extend WorkerSettings with all missing fields

**Files:**
- Modify: `workers/codeforge/config.py`

- [ ] **Step 1: Add `get_settings()` singleton function at end of config.py**

```python
@lru_cache(maxsize=1)
def get_settings() -> WorkerSettings:
    """Return cached WorkerSettings singleton."""
    return WorkerSettings()
```

- [ ] **Step 2: Add core/infrastructure fields to `WorkerSettings.__init__`**

After the existing fields (line 141), add:

```python
        # --- Core / Infrastructure ---
        core_cfg: dict = yaml_cfg.get("core", {}) if isinstance(yaml_cfg.get("core"), dict) else {}
        self.core_url = _resolve_str("CODEFORGE_CORE_URL", core_cfg.get("url"), "http://localhost:8080")
        self.internal_key = _resolve_str("CODEFORGE_INTERNAL_KEY", core_cfg.get("internal_key"), "")
        self.app_env = _resolve_str("APP_ENV", yaml_cfg.get("app_env"), "")
        self.database_url = _resolve_str(
            "DATABASE_URL", yaml_cfg.get("postgres", {}).get("dsn") if isinstance(yaml_cfg.get("postgres"), dict) else None,
            "postgresql://codeforge:codeforge_dev@localhost:5432/codeforge",
        )
        self.workspace = _resolve_str("CODEFORGE_WORKSPACE", None, "/workspaces/CodeForge")
        self.config_file = os.environ.get("CODEFORGE_CONFIG_FILE", "")
```

- [ ] **Step 3: Add LLM fields**

```python
        # --- LLM ---
        self.default_model = _resolve_str("CODEFORGE_DEFAULT_MODEL", litellm_cfg.get("default_model"), "")
```

- [ ] **Step 4: Add consumer fields**

```python
        # --- Consumer ---
        consumer_cfg: dict = yaml_cfg.get("consumer", {}) if isinstance(yaml_cfg.get("consumer"), dict) else {}
        self.consumer_max_errors = _resolve_int("CODEFORGE_CONSUMER_MAX_ERRORS", consumer_cfg.get("max_errors"), 10)
        self.consumer_backoff_multiplier = _resolve_float("CODEFORGE_CONSUMER_BACKOFF_MULTIPLIER", consumer_cfg.get("backoff_multiplier"), 0.5)
        self.consumer_backoff_max = _resolve_float("CODEFORGE_CONSUMER_BACKOFF_MAX", consumer_cfg.get("backoff_max"), 5.0)
```

- [ ] **Step 5: Add Claude Code fields**

```python
        # --- Claude Code ---
        claude_cfg: dict = yaml_cfg.get("claudecode", {}) if isinstance(yaml_cfg.get("claudecode"), dict) else {}
        self.claudecode_enabled = _resolve_bool("CODEFORGE_CLAUDECODE_ENABLED", claude_cfg.get("enabled"), False)
        self.claudecode_path = _resolve_str("CODEFORGE_CLAUDECODE_PATH", claude_cfg.get("path"), "claude")
        self.claudecode_max_concurrent = _resolve_int("CODEFORGE_CLAUDECODE_MAX_CONCURRENT", claude_cfg.get("max_concurrent"), 5)
        self.claudecode_max_turns = _resolve_int("CODEFORGE_CLAUDECODE_MAX_TURNS", claude_cfg.get("max_turns"), 50)
        self.claudecode_timeout = _resolve_int("CODEFORGE_CLAUDECODE_TIMEOUT", claude_cfg.get("timeout"), 300)
        self.claudecode_tiers = _resolve_str("CODEFORGE_CLAUDECODE_TIERS", claude_cfg.get("tiers"), "COMPLEX,REASONING")
```

- [ ] **Step 6: Add routing fields**

```python
        # --- Routing ---
        self.effective_models_cache_ttl = _resolve_float("CODEFORGE_EFFECTIVE_MODELS_CACHE_TTL", routing_cfg.get("effective_models_cache_ttl"), 5.0)
        self.model_block_ttl = _resolve_float("CODEFORGE_MODEL_BLOCK_TTL", routing_cfg.get("model_block_ttl"), 300.0)
        self.model_auth_block_ttl = _resolve_float("CODEFORGE_MODEL_AUTH_BLOCK_TTL", routing_cfg.get("model_auth_block_ttl"), 86400.0)
```

- [ ] **Step 7: Add benchmark fields**

```python
        # --- Benchmark ---
        bench_cfg: dict = yaml_cfg.get("benchmark", {}) if isinstance(yaml_cfg.get("benchmark"), dict) else {}
        self.benchmark_max_parallel = _resolve_int("CODEFORGE_BENCHMARK_MAX_PARALLEL", bench_cfg.get("max_parallel"), 3)
        self.benchmark_datasets_dir = _resolve_str("CODEFORGE_BENCHMARK_DATASETS_DIR", bench_cfg.get("datasets_dir"), "configs/benchmarks")
```

- [ ] **Step 8: Add OTEL fields**

```python
        # --- OpenTelemetry ---
        otel_cfg: dict = yaml_cfg.get("otel", {}) if isinstance(yaml_cfg.get("otel"), dict) else {}
        self.otel_enabled = _resolve_bool("CODEFORGE_OTEL_ENABLED", otel_cfg.get("enabled"), False)
        self.otel_endpoint = _resolve_str("CODEFORGE_OTEL_ENDPOINT", otel_cfg.get("endpoint"), "localhost:4317")
        self.otel_service_name = _resolve_str("CODEFORGE_OTEL_SERVICE_NAME", otel_cfg.get("service_name"), "codeforge-worker")
        self.otel_insecure = _resolve_bool("CODEFORGE_OTEL_INSECURE", otel_cfg.get("insecure"), True)
        self.otel_sample_rate = _resolve_float("CODEFORGE_OTEL_SAMPLE_RATE", otel_cfg.get("sample_rate"), 1.0)
```

- [ ] **Step 9: Add plan/act and evaluation fields**

```python
        # --- Plan/Act ---
        self.plan_act_max_iterations = _resolve_int("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", None, 10)

        # --- Evaluation ---
        eval_cfg: dict = yaml_cfg.get("evaluation", {}) if isinstance(yaml_cfg.get("evaluation"), dict) else {}
        self.judge_model = _resolve_str("CODEFORGE_JUDGE_MODEL", eval_cfg.get("judge_model"), "openai/gpt-4o")
        self.early_stop_threshold = _resolve_float("CODEFORGE_EARLY_STOP_THRESHOLD", eval_cfg.get("early_stop_threshold"), 0.9)
        self.early_stop_quorum = _resolve_int("CODEFORGE_EARLY_STOP_QUORUM", eval_cfg.get("early_stop_quorum"), 3)
        self.hf_token = _resolve_str("HF_TOKEN", None, "")
```

- [ ] **Step 10: Add backend fields**

```python
        # --- Backends ---
        backends_cfg: dict = yaml_cfg.get("backends", {}) if isinstance(yaml_cfg.get("backends"), dict) else {}
        openhands_cfg: dict = backends_cfg.get("openhands", {}) if isinstance(backends_cfg.get("openhands"), dict) else {}
        self.openhands_poll_interval = _resolve_float("CODEFORGE_OPENHANDS_POLL_INTERVAL", openhands_cfg.get("poll_interval"), 2.0)
        self.openhands_http_timeout = _resolve_float("CODEFORGE_OPENHANDS_HTTP_TIMEOUT", openhands_cfg.get("http_timeout"), 30.0)
        self.openhands_health_timeout = _resolve_float("CODEFORGE_OPENHANDS_HEALTH_TIMEOUT", openhands_cfg.get("health_timeout"), 5.0)
        self.openhands_cancel_timeout = _resolve_float("CODEFORGE_OPENHANDS_CANCEL_TIMEOUT", openhands_cfg.get("cancel_timeout"), 5.0)
```

- [ ] **Step 11: Commit**

```bash
git add workers/codeforge/config.py
git commit -m "feat(config): extend WorkerSettings with all env vars + get_settings() singleton"
```

---

### Task 2: Replace scattered reads — core/infrastructure (4 files)

**Files:**
- Modify: `workers/codeforge/agent_loop.py`
- Modify: `workers/codeforge/consumer/_conversation.py`
- Modify: `workers/codeforge/consumer/_compact.py`
- Modify: `workers/codeforge/consumer/_benchmark.py`
- Modify: `workers/codeforge/tools/search_conversations.py`

- [ ] **Step 1: Replace in `agent_loop.py` (lines 1236-1237)**

Replace:
```python
core_url = os.environ.get("CODEFORGE_CORE_URL", "http://localhost:8080")
internal_key = os.environ.get("CODEFORGE_INTERNAL_KEY", "")
```
With:
```python
from codeforge.config import get_settings
_cfg = get_settings()
core_url = _cfg.core_url
internal_key = _cfg.internal_key
```
Remove `os` import if no longer used.

- [ ] **Step 2: Replace in `consumer/_conversation.py` (lines 689-690, 804-805)**

Replace all `os.environ.get("CODEFORGE_CORE_URL", ...)` and `os.environ.get("CODEFORGE_INTERNAL_KEY", ...)` with `get_settings().core_url` and `get_settings().internal_key`.

- [ ] **Step 3: Replace in `consumer/_compact.py` (line 74)**

Replace `os.environ.get("CODEFORGE_CORE_URL", ...)` with `get_settings().core_url`.

- [ ] **Step 4: Replace in `consumer/_benchmark.py` (lines 43, 75, 175, 474, 487)**

Replace:
- `_os.environ.get("CODEFORGE_BENCHMARK_MAX_PARALLEL", ...)` -> `get_settings().benchmark_max_parallel`
- `os.environ.get("LITELLM_MASTER_KEY", ...)` -> `get_settings().litellm_api_key`
- `os.getenv("APP_ENV")` -> `get_settings().app_env`
- `os.environ.get("CODEFORGE_BENCHMARK_DATASETS_DIR", ...)` -> `get_settings().benchmark_datasets_dir`
- `os.environ.get("CODEFORGE_WORKSPACE", ...)` -> `get_settings().workspace`

- [ ] **Step 5: Replace in `tools/search_conversations.py` (line 58)**

Replace `os.environ.get("CODEFORGE_CORE_URL", ...)` with `get_settings().core_url`.

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/agent_loop.py workers/codeforge/consumer/_conversation.py \
  workers/codeforge/consumer/_compact.py workers/codeforge/consumer/_benchmark.py \
  workers/codeforge/tools/search_conversations.py
git commit -m "refactor(worker): centralize core/infra env reads via get_settings()"
```

---

### Task 3: Replace scattered reads — LLM, routing, blocklist (4 files)

**Files:**
- Modify: `workers/codeforge/llm.py`
- Modify: `workers/codeforge/model_resolver.py`
- Modify: `workers/codeforge/routing/router.py`
- Modify: `workers/codeforge/routing/blocklist.py`

- [ ] **Step 1: Replace in `llm.py` (line 28)**

Replace `DEFAULT_MODEL: str = os.environ.get(...)` with:
```python
from codeforge.config import get_settings
DEFAULT_MODEL: str = get_settings().default_model
```

- [ ] **Step 2: Replace in `model_resolver.py` (lines 103-104, 192)**

Replace all `os.environ.get("LITELLM_BASE_URL", ...)`, `os.environ.get("LITELLM_MASTER_KEY", ...)`, `os.environ.get("CODEFORGE_DEFAULT_MODEL", ...)` with `get_settings()` access.

- [ ] **Step 3: Replace in `routing/router.py` (line 48)**

Replace `_EFFECTIVE_MODELS_CACHE_TTL = float(os.environ.get(...))` with `get_settings().effective_models_cache_ttl`.

- [ ] **Step 4: Replace in `routing/blocklist.py` (lines 25-26)**

Replace module-level `os.environ.get(...)` with `get_settings()` access.

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/llm.py workers/codeforge/model_resolver.py \
  workers/codeforge/routing/router.py workers/codeforge/routing/blocklist.py
git commit -m "refactor(worker): centralize LLM/routing env reads via get_settings()"
```

---

### Task 4: Replace scattered reads — Claude Code, plan/act (3 files)

**Files:**
- Modify: `workers/codeforge/claude_code_availability.py`
- Modify: `workers/codeforge/claude_code_executor.py`
- Modify: `workers/codeforge/plan_act.py`

- [ ] **Step 1: Replace in `claude_code_availability.py` (lines 23, 31)**

Replace `os.environ.get("CODEFORGE_CLAUDECODE_ENABLED", ...)` and `CODEFORGE_CLAUDECODE_PATH` with `get_settings()` access.

- [ ] **Step 2: Replace in `claude_code_executor.py` (lines 63, 68, 73, 82)**

Replace all 4 module-level `os.environ.get(...)` calls with `get_settings()` access.

- [ ] **Step 3: Replace in `plan_act.py` (line 30)**

Replace `os.environ.get("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", ...)` with `get_settings().plan_act_max_iterations`.

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/claude_code_availability.py \
  workers/codeforge/claude_code_executor.py workers/codeforge/plan_act.py
git commit -m "refactor(worker): centralize claudecode/plan_act env reads via get_settings()"
```

---

### Task 5: Replace scattered reads — OTEL, evaluation, consumer init (4 files)

**Files:**
- Modify: `workers/codeforge/tracing/setup.py`
- Modify: `workers/codeforge/evaluation/litellm_judge.py`
- Modify: `workers/codeforge/evaluation/cache.py`
- Modify: `workers/codeforge/evaluation/runners/early_stopping.py`
- Modify: `workers/codeforge/consumer/__init__.py`

- [ ] **Step 1: Replace in `tracing/setup.py` (lines 37-43)**

Replace all 5 `os.getenv(...)` calls with `get_settings()` access.

- [ ] **Step 2: Replace in `evaluation/litellm_judge.py` (lines 15, 33-34)**

Replace `os.environ.get("CODEFORGE_JUDGE_MODEL", ...)`, `LITELLM_BASE_URL`, `LITELLM_MASTER_KEY` with `get_settings()` access.

- [ ] **Step 3: Replace in `evaluation/cache.py` (lines 214, 320)**

Replace `os.getenv("HF_TOKEN", ...)` with `get_settings().hf_token`.

- [ ] **Step 4: Replace in `evaluation/runners/early_stopping.py` (lines 45-46)**

Replace `os.environ.get("CODEFORGE_EARLY_STOP_*", ...)` with `get_settings()` access.

- [ ] **Step 5: Replace in `consumer/__init__.py` (lines 89-91, 128-130)**

Replace module-level `os.environ.get(...)` for consumer config and `DATABASE_URL` with `get_settings()` access.

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/tracing/setup.py workers/codeforge/evaluation/litellm_judge.py \
  workers/codeforge/evaluation/cache.py workers/codeforge/evaluation/runners/early_stopping.py \
  workers/codeforge/consumer/__init__.py
git commit -m "refactor(worker): centralize otel/eval/consumer env reads via get_settings()"
```

---

### Task 6: Fix Go remaining `os.Getenv` + fix docs gaps

**Files:**
- Modify: `cmd/codeforge/main.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/loader.go`
- Modify: `docs/dev-setup.md`

- [ ] **Step 1: Add `EnvFile` to Go config struct**

In `internal/config/config.go`, add to the Config struct:
```go
EnvFile string `yaml:"env_file"` // Path to .env file for OAuth device flow
```

- [ ] **Step 2: Add loader line in `internal/config/loader.go`**

```go
setString(&cfg.EnvFile, "CODEFORGE_ENV_FILE")
```

- [ ] **Step 3: Replace in `cmd/codeforge/main.go` (line 536)**

Replace `os.Getenv("CODEFORGE_ENV_FILE")` with `cfg.EnvFile`.

- [ ] **Step 4: Add 17 missing vars to Go Core Config table in `docs/dev-setup.md`**

Add these rows to the Go Core Config table (they exist in Environment Variables table but not in the YAML-mapped table):

```markdown
| `agent.context_enabled` | `CODEFORGE_AGENT_CONTEXT_ENABLED` | `true` | Enable context optimizer |
| `agent.context_budget` | `CODEFORGE_AGENT_CONTEXT_BUDGET` | `2048` | Token budget for context |
| `agent.context_prompt_reserve` | `CODEFORGE_AGENT_CONTEXT_PROMPT_RESERVE` | `512` | Tokens reserved for prompt |
| `agui.enabled` | `CODEFORGE_AGUI_ENABLED` | `false` | Enable AG-UI event emission |
| `copilot.enabled` | `CODEFORGE_COPILOT_ENABLED` | `false` | Enable GitHub Copilot token exchange |
| `experience.enabled` | `CODEFORGE_EXPERIENCE_ENABLED` | `false` | Enable experience pool caching |
| `lsp.enabled` | `CODEFORGE_LSP_ENABLED` | `false` | Enable LSP integration |
| `orchestrator.review_router_enabled` | `CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED` | `false` | Enable review routing |
| `orchestrator.review_confidence_threshold` | `CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD` | `0.7` | Review confidence threshold |
| `orchestrator.review_router_model` | `CODEFORGE_ORCH_REVIEW_ROUTER_MODEL` | `` | Model for review evaluation |
| `quarantine.enabled` | `CODEFORGE_QUARANTINE_ENABLED` | `false` | Enable message quarantine |
| `quarantine.threshold` | `CODEFORGE_QUARANTINE_THRESHOLD` | `0.7` | Risk score for quarantine |
| `quarantine.block_threshold` | `CODEFORGE_QUARANTINE_BLOCK_THRESHOLD` | `0.95` | Risk score for immediate block |
| `quarantine.min_trust_bypass` | `CODEFORGE_QUARANTINE_MIN_TRUST_BYPASS` | `verified` | Min trust level to bypass |
| `quarantine.expiry_hours` | `CODEFORGE_QUARANTINE_EXPIRY_HOURS` | `72` | Hours until expiry |
| `plane.api_token` | `CODEFORGE_PLANE_API_TOKEN` | `` | Plane PM API token |
| `ollama.base_url` | `OLLAMA_BASE_URL` | `` | Ollama local model URL |
```

Also add `CODEFORGE_CONFIG_FILE` to the Python Worker Config table.

- [ ] **Step 5: Commit**

```bash
git add cmd/codeforge/main.go internal/config/config.go internal/config/loader.go docs/dev-setup.md
git commit -m "refactor: centralize last Go os.Getenv + document 17 missing vars"
```

---

### Task 7: Update audit report

**Files:**
- Modify: `docs/audits/config-env-audit.md`

- [ ] **Step 1: Update audit report status**

Update the summary table: set Python scattered reads to FIXED, docs gaps to FIXED, Go remaining to FIXED.

- [ ] **Step 2: Commit**

```bash
git add docs/audits/config-env-audit.md
git commit -m "docs(audit): mark phase 2 centralization as complete"
```

---

## Notes

**NOT centralized (by design):**
- `routing/key_filter.py:66` — dynamic lookup by provider name, cannot be static config
- `backends/*.py` subprocess env propagation — passes env to child processes, not config reading
- `config.py` internal `_resolve_*` helpers — they ARE the config loader
