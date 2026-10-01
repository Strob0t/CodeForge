# E2E Test Setup

Full-stack test setups (moved from the root agent instruction file, now [`AGENTS.md`](../../AGENTS.md), which keeps the short version).

Full stack in **development mode** required:

```bash
# 1. Docker services
docker compose up -d postgres nats litellm
# 2. Go backend (APP_ENV=development required — dev endpoints return 403 without it;
#    CODEFORGE_AUTH_ADMIN_PASS seeds the admin user that Playwright logs in with)
APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/
# 3. Frontend
cd frontend && npm run dev
# 4. Tests
cd frontend && npx playwright test
```

- `/health` exposes `dev_mode: true/false` | Backend: 8080, Frontend: 3000
- Credentials: `admin@localhost` / `Changeme123` (seeded only when `CODEFORGE_AUTH_ADMIN_PASS` is set, otherwise the backend waits for the setup wizard; the admin is created with `must_change_password`, which `frontend/e2e/global-setup.ts` handles) | Playwright: chromium, workers:1, retries:1

## LLM E2E Tests (API-Level, no browser)

```bash
cd frontend && npx playwright test --config=playwright.llm.config.ts
```

88 tests, 11 specs in `frontend/e2e/llm/`. Helper: `frontend/e2e/llm/llm-helpers.ts`. Config: `frontend/playwright.llm.config.ts`

## Autonomous Goal-to-Program Test (Playwright-MCP)

Testplan: `docs/testing/autonomous-goal-to-program-testplan.md` | Tool complexity: `docs/plans/tool-call-complexity-plan.md`

**Critical Startup Sequence (in order):**
1. `docker compose up -d postgres nats litellm`
2. Resolve container IPs (`localhost` ports BROKEN in WSL2):
   ```bash
   NATS_IP=$(docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   LITELLM_IP=$(docker inspect codeforge-litellm | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   POSTGRES_IP=$(docker inspect codeforge-postgres | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   ```
3. Purge NATS JetStream (`await js.purge_stream('CODEFORGE')` + delete stale consumers)
4. Start Go backend: `APP_ENV=development CODEFORGE_AUTH_ADMIN_PASS=Changeme123 go run ./cmd/codeforge/`
5. **VERIFY** toolcall consumer: `curl http://${NATS_IP}:8222/jsz?consumers=1`
6. Start Python worker:
   ```bash
   PYTHONPATH=/workspaces/CodeForge/workers \
     NATS_URL="nats://${NATS_IP}:4222" \
     LITELLM_BASE_URL="http://${LITELLM_IP}:4000" \
     LITELLM_MASTER_KEY="sk-codeforge-dev" \
     DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
     CODEFORGE_ROUTING_ENABLED=false \
     APP_ENV=development \
     .venv/bin/python -m codeforge.consumer
   ```
7. Frontend: `cd frontend && npm run dev`
8. Playwright-MCP browser: `http://host.docker.internal:3000` (not localhost)

**Key env vars:** `LITELLM_BASE_URL` (NOT `LITELLM_URL`), `CODEFORGE_ROUTING_ENABLED=false` (override default=true; avoids router picking unhealthy models in test), auth field: `access_token` (NOT `token`)

**Project setup:**
- Create project: `POST /projects` with `config: {"policy_preset": "trusted-mount-autonomous"}` (the backend also reads `execution_mode`, which only accepts `mount`, and the gate commands `test_command` / `lint_command`, validated on write; autonomy comes from the selected mode; `PUT /projects/{id}` merges config keys, `null` deletes one) and optional `"local_path": "/abs/path"` (auto-adopts workspace)
- Alternatively: `POST /projects/{id}/adopt` with `{"path": "/abs/path"}` as separate call
- TestRepo clone fails often — use local workspace creation instead
- Auto-onboarding disabled (ChatPanel.tsx)
- Model: `"openai/container"` or `"lm_studio/qwen/qwen3-30b-a3b"` (any healthy model)

**Local model timeouts:** 3-10x slower. S1: up to 60min, S4: up to 180min. DO NOT abort early. Monitor via API, not browser. "Stuck" = no new tool calls for 10min.

**HITL:** Poll logs for `"HITL approval requested"`. Approve: `POST /api/v1/runs/{convId}/approve/{callId}` `{"decision":"allow"}`. Bypass: `POST /conversations/{id}/bypass-approvals`
