# Worktree H: Docker Infrastructure Hardening — Atomic Plan

> **Branch:** `fix/docker-infra-hardening`
> **Effort:** ~3h | **Findings:** 12 | **Risk:** Low (config-only, no app code)

---

## Task H1: Pin All Base Images (I-007)

- [ ] `Dockerfile:1`: `FROM golang:1.25-alpine AS build` (already pinned, verify)
- [ ] `Dockerfile.frontend:1`: `FROM node:22-alpine AS build` (already pinned, verify)
- [ ] `Dockerfile.frontend:19`: Change `FROM nginx:alpine` to `FROM nginx:1.27-alpine`
- [ ] `Dockerfile.worker:1`: `FROM python:3.12-slim-bookworm AS build` (already pinned, verify)
- [ ] `docker-compose.yml:9`: Change `image: mcp/playwright:latest` to `image: mcp/playwright:1.50`
- [ ] `docker-compose.yml:24`: Change `image: jaegertracing/all-in-one:latest` to `image: jaegertracing/all-in-one:1.67`
- [ ] `docker-compose.yml:105`: Change `image: ghcr.io/berriai/litellm:main-stable` to `image: ghcr.io/berriai/litellm:v1.63.2`
- [ ] Verify: `docker compose config -q` (no syntax errors)

**Commit:** `fix: pin all Docker base images to specific versions (I-007)`

---

## Task H2: Add cap_drop and security_opt to All Services (I-006)

Add to each service in `docker-compose.prod.yml`:
```yaml
cap_drop:
  - ALL
security_opt:
  - no-new-privileges:true
```

- [ ] `core` service (may already have `security_opt`, add `cap_drop`)
- [ ] `worker` service (may already have `security_opt`, add `cap_drop`)
- [ ] `frontend` service
- [ ] `postgres` service (add `cap_add: [CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID]` for PG init)
- [ ] `nats` service
- [ ] `litellm` service
- [ ] Verify: `docker compose -f docker-compose.prod.yml config -q`

**Commit:** `fix: add cap_drop ALL and no-new-privileges to all prod services (I-006)`

---

## Task H3: Fix PostgreSQL Archive Command (I-010)

**File:** `docker-compose.yml:76`

- [ ] Change from:
```
archive_command='test ! -f /var/lib/postgresql/data/archive/%f && cp %p /var/lib/postgresql/data/archive/%f'
```
To:
```
archive_command='mkdir -p /var/lib/postgresql/data/archive && test ! -f /var/lib/postgresql/data/archive/%f && cp %p /var/lib/postgresql/data/archive/%f'
```
- [ ] Apply same fix in `docker-compose.prod.yml` if present

**Commit:** `fix: add mkdir -p to PostgreSQL archive_command (I-010)`

---

## Task H4: Add Nginx Request Size Limit and Proxy Buffering (I-008, I-009)

**File:** `frontend/nginx.conf`

- [ ] Add inside `server {}` block (after `listen 80;`):
```nginx
client_max_body_size 100M;
```
- [ ] Add to WebSocket location block (`location /ws`):
```nginx
proxy_buffering off;
proxy_request_buffering off;
```
- [ ] Add to API location block (`location /api/`):
```nginx
proxy_buffering off;
```

**Commit:** `fix: add nginx request size limit and disable proxy buffering (I-008, I-009)`

---

## Task H5: Remove Default Credentials from .env.example (I-002)

**File:** `.env.example`

- [ ] Change `POSTGRES_PASSWORD=codeforge_dev` to `POSTGRES_PASSWORD=` with comment `# REQUIRED: set a strong password`
- [ ] Change `LITELLM_MASTER_KEY=sk-codeforge-dev` to `LITELLM_MASTER_KEY=` with comment `# REQUIRED: generate with openssl rand -hex 32`
- [ ] Change `CODEFORGE_INTERNAL_KEY=codeforge-internal-dev` to `CODEFORGE_INTERNAL_KEY=` with comment `# REQUIRED: generate with openssl rand -hex 32`

**Commit:** `fix: remove default credentials from .env.example (I-002)`

---

## Task H6: Configure Traefik TLS Entrypoint (I-013)

**File:** `traefik/traefik.yaml`

- [ ] Add TLS configuration:
```yaml
entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
  websecure:
    address: ":443"
    http:
      tls:
        certResolver: letsencrypt

certificatesResolvers:
  letsencrypt:
    acme:
      email: "${ACME_EMAIL}"
      storage: /acme/acme.json
      tlsChallenge: {}
```
- [ ] Add Traefik security header middleware to `docker-compose.blue-green.yml` (I-018):
```yaml
labels:
  - "traefik.http.middlewares.security.headers.frameDeny=true"
  - "traefik.http.middlewares.security.headers.contentTypeNosniff=true"
  - "traefik.http.middlewares.security.headers.stsSeconds=31536000"
  - "traefik.http.middlewares.security.headers.stsIncludeSubdomains=true"
```
- [ ] Verify: `docker compose -f docker-compose.blue-green.yml config -q`

**Commit:** `fix: configure Traefik TLS with Let's Encrypt and security headers (I-013, I-018)`

---

## Task H7: Increase Log Retention (I-014)

**Files:** `docker-compose.yml`, `docker-compose.prod.yml`

- [ ] Change logging config from:
```yaml
options:
  max-size: "10m"
  max-file: "3"
```
To:
```yaml
options:
  max-size: "50m"
  max-file: "10"
```

**Commit:** `fix: increase Docker log retention to 50m x 10 files (I-014)`

---

## Task H8: Pin GitHub Action Versions (I-019)

**File:** `.github/workflows/ci.yml`

- [ ] Change `golangci/golangci-lint-action@v7` to `golangci/golangci-lint-action@v7.0.0`
- [ ] Change `treosh/lighthouse-ci-action@v12` to `treosh/lighthouse-ci-action@v12.1.0`
- [ ] Verify: check latest versions and pin to current

**Commit:** `fix: pin GitHub Action versions to specific semver (I-019)`

---

## Task H9: Isolate Playwright-MCP (I-017)

**File:** `docker-compose.yml`

- [ ] Add `profiles: ["dev"]` to playwright-mcp service (already dev-only but not profiled)
- [ ] Document `--no-sandbox` rationale in comment

**Commit:** `fix: add dev profile to playwright-mcp service (I-017)`

---

## Task H10: Add Pre-deploy Env Validation Script (I-015)

**Create:** `scripts/validate-env.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
# Validates required env vars before production deployment.
REQUIRED=(POSTGRES_PASSWORD LITELLM_MASTER_KEY CODEFORGE_INTERNAL_KEY CODEFORGE_JWT_SECRET)
for var in "${REQUIRED[@]}"; do
  if [ -z "${!var:-}" ]; then
    echo "ERROR: $var is not set" >&2; exit 1
  fi
  if [[ "${!var}" =~ (codeforge_dev|sk-codeforge-dev|codeforge-internal-dev) ]]; then
    echo "ERROR: $var contains a default/insecure value" >&2; exit 1
  fi
done
echo "All required env vars validated."
```

- [ ] `chmod +x scripts/validate-env.sh`
- [ ] Verify: script runs and validates correctly

**Commit:** `feat: add pre-deploy env validation script (I-015)`

---

## Verification

- [ ] `docker compose config -q`
- [ ] `docker compose -f docker-compose.prod.yml config -q`
- [ ] `docker compose -f docker-compose.blue-green.yml config -q`
