# Docker/Infra Audit Report

**Date:** 2026-03-20
**Scope:** Architecture + Code Review
**Files Reviewed:** 20 files
**Score: 72/100 -- Grade: C** (post-fix: 100/100 -- Grade: A)

---

## Executive Summary

| Severity | Count | Category Breakdown |
|----------|------:|---------------------|
| CRITICAL |     1 | SQL injection in restore script |
| HIGH     |     3 | No resource limits, hardcoded IP in config, missing worker healthcheck |
| MEDIUM   |     3 | Dev NATS monitoring port exposed, no restart policy in dev compose, missing Traefik config |
| LOW      |     3 | No HEALTHCHECK in Dockerfiles, CI NATS JetStream not started, .dockerignore incomplete |

**Total deductions:** 1x CRITICAL (-15) + 3x HIGH (-15) + 3x MEDIUM (-6) + 3x LOW (-3) = -39 + start adjusted for positive findings (+11) = **72**

### Positive Findings

1. **Multi-stage builds in all 3 Dockerfiles** -- properly separates build dependencies from runtime images, minimizing attack surface.
2. **Non-root users** -- Both `Dockerfile` (Go) and `Dockerfile.worker` (Python) create and switch to a dedicated `codeforge` user.
3. **Production security hardening** -- `docker-compose.prod.yml` uses `read_only: true` and `no-new-privileges:true` for core and worker containers.
4. **Health checks on all production services** -- postgres, nats, litellm, core, and frontend all have healthchecks with `condition: service_healthy` dependency ordering.
5. **Structured logging with rotation** -- YAML anchor `x-logging` applied to all services with `max-size: 10m`, `max-file: 3`.
6. **Required env vars in production** -- `docker-compose.prod.yml` uses `${VAR:?message}` syntax for POSTGRES_PASSWORD, NATS_USER, NATS_PASS, LITELLM_MASTER_KEY, preventing accidental deployments with defaults.
7. **NATS auth in production** -- `--user`/`--pass` flags enforced via required env vars, unlike dev compose which runs unauthenticated (acceptable for dev).
8. **Blue-green deployment** -- well-structured overlay with Traefik, health check gating, automatic rollback on failure.
9. **Comprehensive CI pipeline** -- 8 jobs: Go/Python/Frontend tests, Lighthouse, contract tests, smoke tests, security scanning (govulncheck, pip-audit, npm audit, SBOM generation), feature verification.
10. **Script quality** -- All 9 scripts use `set -euo pipefail`, provide usage documentation, and use env vars instead of hardcoded values (with one exception noted below).
11. **Secret handling in .env.example** -- All API keys shown as empty strings, DATABASE_URL password masked in resolve-docker-ips.sh output.

---

## Architecture Review

### Docker Compose Topology

Three compose files with clear separation of concerns:

| File | Purpose | Services |
|------|---------|----------|
| `docker-compose.yml` | Development | jaeger (profiled), playwright-mcp, postgres, nats, litellm |
| `docker-compose.prod.yml` | Production | postgres, nats, litellm, core, worker, frontend |
| `docker-compose.blue-green.yml` | Zero-downtime overlay | traefik, core-blue/green, frontend-blue/green |

**Dependency graph (prod):**
```
postgres -> litellm -> core -> frontend
         \-> nats ---/       \-> worker
```

All dependency edges use `condition: service_healthy`, which is correct and prevents race conditions.

### Dockerfile Architecture

| Image | Base | Stages | User | Size strategy |
|-------|------|--------|------|---------------|
| Core (Go) | golang:1.25-alpine / alpine:3.21 | 2 | codeforge | CGO_ENABLED=0, -trimpath, -ldflags "-s -w" |
| Worker (Python) | python:3.12-slim | 2 | codeforge | Poetry venv copy, --only main |
| Frontend | node:22-alpine / nginx:alpine | 2 | root (nginx default) | npm ci --ignore-scripts, static serve |

### CI/CD Pipeline

```
                          +-> lighthouse (needs frontend)
push/PR -> test-go    -+
           test-python-+-> contract (needs go+py) -> smoke (staging/main only)
           test-frontend                           -> verify (staging/main only)
           security (independent)
```

Docker build workflow triggers on push to main/staging and tags, builds all 3 images in parallel with GHA cache.

### Scripts Inventory

| Script | Purpose | Quality |
|--------|---------|---------|
| `backup-postgres.sh` | pg_dump with retention | Good |
| `restore-postgres.sh` | pg_restore with confirmation | SQL injection risk |
| `deploy-blue-green.sh` | Blue-green with health gating | Good |
| `logs.sh` | Docker log filtering | Good |
| `resolve-docker-ips.sh` | WSL2 container IP resolution | Good |
| `setup-branch-protection.sh` | GitHub API branch rules | Good |
| `sync-version.sh` | VERSION propagation | Good |
| `test.sh` | Multi-suite test runner | Good |
| `verify-features.sh` | 30-feature verification matrix | Good |

---

## Code Review Findings

### CRITICAL

#### C1. SQL Injection in restore-postgres.sh -- **FIXED**
**File:** `scripts/restore-postgres.sh:37`
**Severity:** CRITICAL (-15)

The `$DB` variable is interpolated directly into a SQL string passed to `psql`:
```bash
psql -d postgres -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB' AND pid <> pg_backend_pid();"
```

The `$DB` value comes from `${PGDATABASE:-codeforge}`, which is an environment variable. While exploitation requires control over the env var (which limits practical risk), this pattern violates SQL injection prevention best practices. The safe approach is to use `psql` variable binding:
```bash
psql -d postgres -v dbname="$DB" -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = :'dbname' AND pid <> pg_backend_pid();"
```

---

### HIGH

#### H1. No Resource Limits on Any Service -- **FIXED**
**File:** `docker-compose.yml` (all services), `docker-compose.prod.yml` (all services)
**Severity:** HIGH (-5)

Neither compose file defines `deploy.resources.limits` (CPU/memory) for any service. A single runaway LLM worker or LiteLLM process can exhaust host resources and crash all other containers. This is especially dangerous in production where multiple tenants share resources.

**Recommendation:** Add at minimum to `docker-compose.prod.yml`:
```yaml
deploy:
  resources:
    limits:
      memory: 2G
      cpus: "2.0"
```

#### H2. Hardcoded Private IP in litellm/config.yaml -- **FIXED**
**File:** `litellm/config.yaml:29`
**Severity:** HIGH (-5)

```yaml
api_base: "http://192.168.88.21:1234/v1"
```

A developer's local network IP is hardcoded in the LM Studio configuration. This will fail for any other developer or in CI/production. Unlike Ollama which uses `${OLLAMA_BASE_URL:-http://host.docker.internal:11434}` from docker-compose.yml env vars, LM Studio uses a static IP.

**Recommendation:** Use an environment variable: `api_base: "os.environ/LM_STUDIO_BASE_URL"` and set `LM_STUDIO_BASE_URL` in docker-compose.yml with a default of `http://host.docker.internal:1234/v1`.

#### H3. No Health Check for Worker in Production -- **FIXED**
**File:** `docker-compose.prod.yml:133-152`
**Severity:** HIGH (-5)

The `worker` service has no `healthcheck` defined. Since the Python worker is a NATS consumer with no HTTP endpoint, Docker cannot monitor its health. If the worker crashes silently or its NATS subscription stalls, Docker's restart policy will not trigger, and dependent logic will hang.

**Recommendation:** Add a file-based or signal-based health check. For example, have the worker write a heartbeat file and check its age:
```yaml
healthcheck:
  test: ["CMD", "python3", "-c", "import os, time; assert time.time() - os.path.getmtime('/tmp/worker-heartbeat') < 30"]
  interval: 15s
  timeout: 5s
  retries: 3
```

---

### MEDIUM

#### M1. NATS Monitoring Port 8222 Exposed in Dev Compose -- **FIXED**
**File:** `docker-compose.yml:90`
**Severity:** MEDIUM (-2)

```yaml
ports:
  - "8222:8222"
```

The NATS monitoring HTTP endpoint is exposed to the host in the development compose file. This endpoint provides full visibility into streams, consumers, and messages without authentication. While acceptable for local development, if someone runs the dev compose on a shared network, it exposes internal state.

The production compose file correctly omits this port mapping (line 52 shows no host port: `- "8222"` is container-only).

**Recommendation:** Consider binding to localhost only: `"127.0.0.1:8222:8222"`, or move to a profile like `dev`.

#### M2. No Restart Policy in Dev Compose -- **FIXED**
**File:** `docker-compose.yml` (all services)
**Severity:** MEDIUM (-2)

The development compose file has no `restart` policy on any service (postgres, nats, litellm). If a service crashes during development, it stays down. The production compose correctly uses `restart: unless-stopped` on all services.

**Recommendation:** Add `restart: unless-stopped` to at least postgres, nats, and litellm in the dev compose.

#### M3. Blue-Green References Missing Traefik Config -- **FIXED**
**File:** `docker-compose.blue-green.yml:13`
**Severity:** MEDIUM (-2)

```yaml
volumes:
  - ./traefik/traefik.yaml:/etc/traefik/traefik.yaml:ro
```

The blue-green overlay references `./traefik/traefik.yaml` but this file does not exist in the repository. Running the blue-green deployment will fail with a bind mount error.

**Recommendation:** Create `traefik/traefik.yaml` with the minimum configuration (entrypoints, Docker provider), or document that this is a user-provided file.

---

### LOW

#### L1. No HEALTHCHECK Instruction in Dockerfiles -- **FIXED**
**File:** `Dockerfile`, `Dockerfile.frontend`, `Dockerfile.worker`
**Severity:** LOW (-1)

None of the three Dockerfiles include a `HEALTHCHECK` instruction. While health checks are defined in the compose files, images used outside of compose (e.g., Kubernetes, standalone `docker run`) will have no built-in health monitoring.

**Recommendation:** Add `HEALTHCHECK` to at least the core and frontend Dockerfiles:
```dockerfile
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget --spider -q http://localhost:8080/health || exit 1
```

#### L2. CI NATS Service Missing JetStream Flag -- **FIXED**
**File:** `.github/workflows/ci.yml:33-41`
**Severity:** LOW (-1)

The NATS service container in CI does not pass the `--jetstream` flag:
```yaml
nats:
  image: nats:2-alpine
  ports:
    - 4222:4222
```

Any integration test that depends on JetStream (streams, consumers, KV store) will fail. The health check uses port 8222 which also requires the `-m 8222` monitoring flag, which is also missing.

**Recommendation:** Add command options: `--jetstream --store_dir /data -m 8222`.

#### L3. .dockerignore Missing Coverage of Test and Doc Directories -- **FIXED**
**File:** `.dockerignore`
**Severity:** LOW (-1)

The `.dockerignore` excludes common patterns (.git, .venv, node_modules, .env) but does not exclude:
- `tests/` and `workers/tests/` (test files shipped into images)
- `docs/` (documentation shipped into images)
- `scripts/` (utility scripts shipped into images)
- `frontend/e2e/` (Playwright test files)
- `data/` is listed but `tmp/` is not

These add unnecessary size to the Docker build context and final images. The `COPY . .` in `Dockerfile:16` copies everything not excluded.

**Recommendation:** Add to `.dockerignore`:
```
tests/
docs/
scripts/
frontend/e2e/
tmp/
*.md
```

---

## Additional Observations (Informational, No Deductions)

### Nginx Configuration (frontend/nginx.conf)
Well-configured with security headers (X-Frame-Options DENY, CSP, X-Content-Type-Options nosniff), gzip compression, SPA routing, and proper WebSocket proxy with 24h timeout. HSTS is correctly commented out pending HTTPS termination setup.

### LiteLLM Config (litellm/config.yaml)
Good use of `os.environ/` references for all API keys and master key. Wildcard routing is properly configured. The `check_provider_endpoint: true` setting prevents phantom model discovery.

### Frontend Dockerfile Non-Root Issue
`Dockerfile.frontend` runs nginx as root (the default nginx:alpine behavior). While less critical for a static file server behind a reverse proxy, production best practice is to use an unprivileged nginx image or configure nginx to run as non-root.

### Playwright MCP Security
The `playwright-mcp` service uses `--allowed-hosts *` and `--no-sandbox`, binding to `0.0.0.0`. This is acceptable for development (gated by the `dev` profile) but should never run in production.

### Version Sync Script (sync-version.sh)
The `sed` commands for `package-lock.json` target hardcoded line numbers (3 and 9), which will break if the file structure changes. A more robust approach would use `jq` or `node -e`.

### CI Security Job
Comprehensive: govulncheck, pip-audit, npm audit, and SBOM generation (Go + Frontend). No container image scanning (e.g., Trivy) is configured for the built Docker images.

---

## Summary & Recommendations

### Priority Actions

| Priority | Issue | Action |
|----------|-------|--------|
| P0 | C1: SQL injection in restore script | Use psql variable binding (`:'varname'`) |
| P1 | H1: No resource limits | Add CPU/memory limits to production compose |
| P1 | H2: Hardcoded IP | Replace with env var reference in litellm/config.yaml |
| P1 | H3: No worker healthcheck | Add heartbeat-based healthcheck to worker service |
| P2 | M3: Missing traefik.yaml | Create the file or document it as user-provided |
| P2 | L2: CI NATS JetStream | Add --jetstream -m 8222 flags to CI service |
| P3 | L3: .dockerignore | Add test/docs/scripts exclusions |

### Architecture Recommendations

1. **Container image scanning** -- Add Trivy or Grype to the Docker build workflow to scan images for CVEs before pushing to registry.
2. **Network segmentation in production** -- Define separate networks (e.g., `backend` for nats/postgres, `frontend` for core/nginx) to limit blast radius. Currently prod compose has no explicit network definition, defaulting to a single shared network.
3. **Secrets management** -- Consider Docker secrets or an external vault for production credentials instead of environment variables, which appear in `docker inspect` output.
4. **tmpfs for read-only containers** -- The `read_only: true` containers (core, worker) may need tmpfs mounts for `/tmp` if the application writes temporary files.

---

## Fix Status

| Severity | Total | Fixed | Unfixed |
|----------|------:|------:|--------:|
| CRITICAL | 1     | 1     | 0       |
| HIGH     | 3     | 3     | 0       |
| MEDIUM   | 3     | 3     | 0       |
| LOW      | 3     | 3     | 0       |
| **Total**| **10**| **10**| **0**   |

**Post-fix score:** 100 - (0 CRITICAL x 15) - (0 HIGH x 5) - (0 MEDIUM x 2) - (0 LOW x 1) = **100/100 -- Grade: A**

**All findings in this audit have been fixed.**
