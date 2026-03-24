# WT-12: Infrastructure & Production Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-request wall-clock timeout and memory monitoring to Python worker, implement Docker Secrets support for production secret management with env-var fallback for development.

**Architecture:** Worker gets `asyncio.wait_for()` timeout wrapper + `psutil` memory checks. Secrets use a provider pattern: Docker Secrets (`/run/secrets/*`) in production, env vars in development. Go Core reads secrets via a `SecretsProvider` interface.

**Tech Stack:** Python 3.12, asyncio, psutil, Docker Compose secrets, Go 1.25

**Best Practice:**
- 12-Factor App: Store config in the environment, but secrets should use dedicated secret stores.
- Docker Secrets: File-based (`/run/secrets/*`), not visible in `docker inspect`, read-only mount.
- Python asyncio: Use `asyncio.wait_for()` for hard wall-clock timeouts on async operations.
- Memory monitoring: Check RSS between expensive operations, gracefully reject new work if threshold exceeded.

**Research Findings:**
- LLM call timeouts already implemented: httpx 10s connect, 300s read (configurable)
- Loop iteration limits: 50 max (configurable). Cost limits supported.
- Tool output truncation: 10K chars head-tail. Token limits per model capability.
- Container limits: 4G memory in prod compose. **Gap:** No per-request memory monitoring, no hard wall-clock timeout.
- Secrets: Go `internal/secrets/vault.go` exists but only used for log redaction, not for actual secret management.

---

### Task 1: Add Wall-Clock Timeout to Agent Loop

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Wrap agent loop execution with asyncio.wait_for()**

In the conversation handler where `AgentLoopExecutor.run()` is called, add a configurable wall-clock timeout:

```python
import asyncio

CONVERSATION_TIMEOUT = int(os.getenv("CODEFORGE_CONVERSATION_TIMEOUT", "3600"))  # 1 hour default

async def _execute_conversation_run(self, ...):
    executor = AgentLoopExecutor(...)
    try:
        result = await asyncio.wait_for(
            executor.run(),
            timeout=CONVERSATION_TIMEOUT,
        )
    except asyncio.TimeoutError:
        logger.warning("conversation timed out", conversation_id=conv_id, timeout=CONVERSATION_TIMEOUT)
        # Publish timeout completion event
        await self._publish_completion(conv_id, status="timeout", error="Wall-clock timeout exceeded")
        return
```

- [ ] **Step 2: Add CODEFORGE_CONVERSATION_TIMEOUT to config**

Document in `.env.example`:
```env
# Maximum wall-clock time for a single conversation run (seconds, default: 3600)
CODEFORGE_CONVERSATION_TIMEOUT=3600
```

- [ ] **Step 3: Run tests**

```bash
.venv/bin/python -m pytest workers/tests/consumer/ -v
```

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/consumer/_conversation.py .env.example
git commit -m "feat: add wall-clock timeout to conversation runs (default 1h)"
```

---

### Task 2: Add Memory Monitoring Between Tool Calls

**Files:**
- Modify: `workers/codeforge/agent_loop.py`
- Modify: `pyproject.toml` (add psutil dependency)

- [ ] **Step 1: Add psutil to dependencies**

```bash
cd /workspaces/CodeForge && poetry add psutil
```

- [ ] **Step 2: Add memory check in agent loop iteration**

```python
import psutil

MEMORY_THRESHOLD_MB = int(os.getenv("CODEFORGE_WORKER_MEMORY_THRESHOLD_MB", "3500"))

def _check_memory(self) -> bool:
    """Return True if memory usage is within acceptable limits."""
    process = psutil.Process()
    rss_mb = process.memory_info().rss / (1024 * 1024)
    if rss_mb > MEMORY_THRESHOLD_MB:
        logger.warning("memory threshold exceeded", rss_mb=rss_mb, threshold=MEMORY_THRESHOLD_MB)
        return False
    return True
```

Add to the main loop iteration (before each tool call):
```python
if not self._check_memory():
    logger.error("aborting run due to memory pressure", rss_mb=rss_mb)
    return LoopResult(status="error", error="Memory threshold exceeded")
```

- [ ] **Step 3: Run tests + commit**

```bash
.venv/bin/python -m pytest workers/tests/ -v
git add workers/codeforge/agent_loop.py pyproject.toml poetry.lock
git commit -m "feat: add memory monitoring with configurable threshold in agent loop"
```

---

### Task 3: Implement Docker Secrets Support for Go Core

**Files:**
- Create: `internal/secrets/provider.go`
- Modify: `internal/config/config.go`
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Create SecretsProvider interface**

```go
// internal/secrets/provider.go
package secrets

import (
    "fmt"
    "os"
    "strings"
)

// Provider abstracts secret retrieval. Implementations:
// - EnvProvider: reads from environment variables (development)
// - FileProvider: reads from Docker Secrets files (production)
type Provider interface {
    Get(key string) (string, error)
}

// EnvProvider reads secrets from environment variables.
type EnvProvider struct{}

func (EnvProvider) Get(key string) (string, error) {
    v := os.Getenv(key)
    if v == "" {
        return "", fmt.Errorf("env var %s not set", key)
    }
    return v, nil
}

// FileProvider reads secrets from /run/secrets/ (Docker Secrets).
// Falls back to environment variables if file doesn't exist.
type FileProvider struct {
    Dir string // default: /run/secrets
}

func NewFileProvider() *FileProvider {
    dir := os.Getenv("DOCKER_SECRETS_DIR")
    if dir == "" {
        dir = "/run/secrets"
    }
    return &FileProvider{Dir: dir}
}

func (fp *FileProvider) Get(key string) (string, error) {
    // Convert KEY_NAME to key_name for file lookup
    fileName := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
    path := fp.Dir + "/" + fileName
    data, err := os.ReadFile(path)
    if err != nil {
        // Fallback to env var
        if v := os.Getenv(key); v != "" {
            return v, nil
        }
        return "", fmt.Errorf("secret %s: file %s not found and env var not set", key, path)
    }
    return strings.TrimSpace(string(data)), nil
}

// Auto selects FileProvider if /run/secrets exists, else EnvProvider.
func Auto() Provider {
    if info, err := os.Stat("/run/secrets"); err == nil && info.IsDir() {
        return NewFileProvider()
    }
    return EnvProvider{}
}
```

- [ ] **Step 2: Wire into main.go**

```go
secretProvider := secrets.Auto()
litellmKey, _ := secretProvider.Get("LITELLM_MASTER_KEY")
postgresPass, _ := secretProvider.Get("POSTGRES_PASSWORD")
```

- [ ] **Step 3: Run tests + commit**

```bash
go test ./internal/secrets/... -v
git add internal/secrets/ cmd/codeforge/main.go
git commit -m "feat: add SecretsProvider with Docker Secrets + env var fallback (F-053)"
```

---

### Task 4: Implement Docker Secrets Support for Python Worker

**Files:**
- Create: `workers/codeforge/secrets.py`
- Modify: `workers/codeforge/consumer/__init__.py`

- [ ] **Step 1: Create secrets module**

```python
# workers/codeforge/secrets.py
"""Secret provider with Docker Secrets file fallback to env vars."""

import os
from pathlib import Path

import structlog

logger = structlog.get_logger(component="secrets")

SECRETS_DIR = Path(os.getenv("DOCKER_SECRETS_DIR", "/run/secrets"))


def get_secret(key: str, default: str = "") -> str:
    """Read secret from Docker Secrets file, fall back to env var."""
    file_path = SECRETS_DIR / key.lower().replace("_", "-")
    if file_path.is_file():
        value = file_path.read_text().strip()
        logger.debug("loaded secret from file", key=key)
        return value
    value = os.getenv(key, default)
    if value:
        logger.debug("loaded secret from env", key=key)
    return value
```

- [ ] **Step 2: Use in consumer initialization**

```python
from codeforge.secrets import get_secret

litellm_key = get_secret("LITELLM_MASTER_KEY")
database_url = get_secret("DATABASE_URL")
```

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/secrets.py workers/codeforge/consumer/__init__.py
git commit -m "feat: add Python secrets provider with Docker Secrets support (F-053)"
```

---

### Task 5: Update docker-compose.prod.yml with Secrets

**Files:**
- Modify: `docker-compose.prod.yml`
- Create: `scripts/generate-secrets.sh`

- [ ] **Step 1: Add secrets block to prod compose**

```yaml
secrets:
  litellm-master-key:
    file: ${SECRETS_DIR:-./secrets}/litellm-master-key
  postgres-password:
    file: ${SECRETS_DIR:-./secrets}/postgres-password
  nats-user:
    file: ${SECRETS_DIR:-./secrets}/nats-user
  nats-pass:
    file: ${SECRETS_DIR:-./secrets}/nats-pass

services:
  core:
    secrets:
      - litellm-master-key
      - postgres-password
      - nats-user
      - nats-pass

  worker:
    secrets:
      - litellm-master-key
      - postgres-password
      - nats-user
      - nats-pass

  litellm:
    secrets:
      - litellm-master-key
      - postgres-password
```

- [ ] **Step 2: Create secret generation script**

```bash
#!/usr/bin/env bash
# scripts/generate-secrets.sh — Generate random secrets for production
set -euo pipefail

SECRETS_DIR="${1:-./secrets}"
mkdir -p "$SECRETS_DIR"

generate() {
    local name="$1"
    local file="$SECRETS_DIR/$name"
    if [ -f "$file" ]; then
        echo "Secret $name already exists, skipping"
        return
    fi
    openssl rand -base64 32 | tr -d '\n' > "$file"
    chmod 600 "$file"
    echo "Generated $name"
}

generate litellm-master-key
generate postgres-password
generate nats-user
generate nats-pass

echo "Secrets generated in $SECRETS_DIR"
```

- [ ] **Step 3: Verify compose config**

```bash
docker compose -f docker-compose.prod.yml config --quiet
```

- [ ] **Step 4: Commit**

```bash
chmod +x scripts/generate-secrets.sh
git add docker-compose.prod.yml scripts/generate-secrets.sh
git commit -m "feat: add Docker Secrets to prod compose with generation script (F-053)"
```

---

### Task 6: Document Production Secret Management

**Files:**
- Modify: `docs/SECURITY.md`
- Modify: `docs/dev-setup.md`

- [ ] **Step 1: Add secrets section to SECURITY.md**

```markdown
## Secret Management

### Development
Secrets are loaded from environment variables (`.env` file, gitignored).

### Production
Secrets are stored as Docker Secrets in `/run/secrets/`:
1. Generate: `./scripts/generate-secrets.sh ./secrets`
2. Deploy: `docker compose -f docker-compose.prod.yml up -d`
3. Rotate: Update secret file, recreate service

### Hierarchy
1. Docker Secrets (`/run/secrets/*`) — production
2. Environment variables — development/fallback
3. Config file defaults — NEVER for secrets
```

- [ ] **Step 2: Commit**

```bash
git add docs/SECURITY.md docs/dev-setup.md
git commit -m "docs: document production secret management (Docker Secrets)"
```
