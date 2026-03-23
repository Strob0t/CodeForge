# WT-5: Docker & Infrastructure Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden Docker Compose dev/prod configs: resource limits, Playwright security, Traefik TLS/rate-limiting, archive retention, NATS monitoring protection.

**Architecture:** Configuration-only changes to Docker Compose files, Traefik config, and a WAL archive cleanup script.

**Tech Stack:** Docker Compose, Traefik v3, NATS, PostgreSQL

**Best Practice:**
- CIS Docker Benchmark: Drop all capabilities, add only required ones. Run as non-root.
- Resource limits: Always set memory limits to prevent OOM cascades.
- Traefik: Use Let's Encrypt ACME for TLS, middleware for rate limiting.
- PostgreSQL: WAL archive rotation prevents disk exhaustion.

---

### Task 1: Add Resource Limits to Dev Compose

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add deploy.resources.limits to each service**

Add after `networks:` block for each service:

```yaml
# postgres (line ~85, after networks)
    deploy:
      resources:
        limits:
          memory: 1G

# nats (line ~104, after networks)
    deploy:
      resources:
        limits:
          memory: 512M

# litellm (line ~175, after networks)
    deploy:
      resources:
        limits:
          memory: 2G

# playwright-mcp (line ~54, after networks — already has dev profile)
    deploy:
      resources:
        limits:
          memory: 1G

# docs-mcp already has limits (512M) - no change needed
```

- [ ] **Step 2: Verify compose config is valid**

```bash
docker compose config --quiet
```

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "fix: add resource limits to dev docker-compose services"
```

---

### Task 2: Harden Playwright MCP

**Files:**
- Modify: `docker-compose.yml:23-54`

- [ ] **Step 1: Remove --no-sandbox, add user and security_opt**

Replace the playwright-mcp service command and add security directives:
```yaml
  playwright-mcp:
    image: mcp/playwright:latest
    container_name: codeforge-playwright
    logging: *default-logging
    profiles:
      - dev
    ipc: host
    shm_size: "1gb"
    user: "1000:1000"
    security_opt:
      - "no-new-privileges:true"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "8001:8001"
    networks:
      - codeforge
    command:
      - "--browser"
      - "chromium"
      - "--isolated"
      - "--headless"
      - "--port"
      - "8001"
      - "--host"
      - "0.0.0.0"
      - "--allowed-hosts"
      - "*"
```

Note: `--no-sandbox` removed. If Chromium fails without it due to missing capabilities, add `cap_add: [SYS_ADMIN]` instead.

- [ ] **Step 2: Test Playwright still works**

```bash
docker compose --profile dev up playwright-mcp -d
docker compose --profile dev logs playwright-mcp | tail -5
```
If fails with sandbox error, add `cap_add: [SYS_ADMIN]`.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "fix: harden Playwright MCP — remove --no-sandbox, add user/security_opt"
```

---

### Task 3: Add Security Baseline to Dev Compose

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add security_opt and cap_drop to all dev services**

For postgres, nats, litellm, docs-mcp, add:
```yaml
    cap_drop:
      - ALL
    security_opt:
      - "no-new-privileges:true"
```

For postgres, also add back required caps:
```yaml
    cap_add:
      - CHOWN
      - DAC_OVERRIDE
      - FOWNER
      - SETGID
      - SETUID
```
(Matching the prod compose pattern.)

- [ ] **Step 2: Verify all services start**

```bash
docker compose up -d postgres nats litellm
docker compose ps
```

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "fix: add security baseline (cap_drop, no-new-privileges) to dev compose"
```

---

### Task 4: WAL Archive Retention Script

**Files:**
- Create: `scripts/cleanup-wal-archives.sh`
- Modify: `docker-compose.prod.yml` (document usage)

- [ ] **Step 1: Create cleanup script**

```bash
#!/usr/bin/env bash
# scripts/cleanup-wal-archives.sh
# Remove PostgreSQL WAL archives older than RETENTION_DAYS (default: 7)
set -euo pipefail

RETENTION_DAYS="${1:-7}"
ARCHIVE_DIR="${ARCHIVE_DIR:-/archive}"

if [ ! -d "$ARCHIVE_DIR" ]; then
    echo "Archive directory $ARCHIVE_DIR not found"
    exit 0
fi

COUNT=$(find "$ARCHIVE_DIR" -name "*.backup" -o -name "0000*" -mtime +"$RETENTION_DAYS" | wc -l)
if [ "$COUNT" -gt 0 ]; then
    find "$ARCHIVE_DIR" -name "*.backup" -o -name "0000*" -mtime +"$RETENTION_DAYS" -delete
    echo "Cleaned up $COUNT WAL archive files older than $RETENTION_DAYS days"
else
    echo "No WAL archives older than $RETENTION_DAYS days"
fi
```

- [ ] **Step 2: Make executable**

```bash
chmod +x scripts/cleanup-wal-archives.sh
```

- [ ] **Step 3: Commit**

```bash
git add scripts/cleanup-wal-archives.sh
git commit -m "feat: add WAL archive retention cleanup script"
```

---

### Task 5: Traefik TLS and Rate Limiting

**Files:**
- Modify: `traefik/traefik.yaml`
- Create: `traefik/dynamic/middleware.yaml`

- [ ] **Step 1: Update Traefik static config**

```yaml
api:
  dashboard: false

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

certificatesResolvers:
  letsencrypt:
    acme:
      email: "${ACME_EMAIL:-admin@localhost}"
      storage: /acme/acme.json
      httpChallenge:
        entryPoint: web

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
    watch: true
  file:
    directory: "/etc/traefik/dynamic"
    watch: true

accessLog:
  format: json
  fields:
    headers:
      defaultMode: drop
      names:
        User-Agent: keep
        Authorization: redact

log:
  level: INFO
  format: json
```

- [ ] **Step 2: Create rate limiting middleware**

```yaml
# traefik/dynamic/middleware.yaml
http:
  middlewares:
    rate-limit:
      rateLimit:
        average: 100
        burst: 200
        period: 1s
    security-headers:
      headers:
        stsSeconds: 31536000
        stsIncludeSubdomains: true
        forceSTSHeader: true
        frameDeny: true
        contentTypeNosniff: true
        browserXssFilter: true
```

- [ ] **Step 3: Commit**

```bash
git add traefik/
git commit -m "feat: configure Traefik TLS (ACME), rate limiting, and access logs"
```

---

### Task 6: Protect NATS Monitoring in Production

**Files:**
- Modify: `docker-compose.prod.yml`

- [ ] **Step 1: Disable NATS monitoring port in production**

Remove `-m 8222` from the NATS command in prod compose, or bind monitoring to localhost only. Since prod uses internal-only network, the simplest approach is to remove the monitoring flag:

```yaml
  nats:
    command:
      - "--jetstream"
      - "--store_dir"
      - "/data"
      - "--user"
      - "${NATS_USER:?NATS_USER is required}"
      - "--pass"
      - "${NATS_PASS:?NATS_PASS is required}"
    # Monitoring removed in prod — use NATS CLI for debugging
```

- [ ] **Step 2: Update healthcheck to not depend on monitoring port**

```yaml
    healthcheck:
      test: ["CMD", "nats-server", "--signal", "ldm"]
      interval: 10s
      timeout: 5s
      retries: 5
```

Or use a simple TCP check:
```yaml
    healthcheck:
      test: ["CMD", "sh", "-c", "echo | nc -z localhost 4222"]
```

- [ ] **Step 3: Commit**

```bash
git add docker-compose.prod.yml
git commit -m "fix: disable NATS monitoring endpoint in production"
```
