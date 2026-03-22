# Full Service Interactive QA Report

**Date:** 2026-03-22
**Testplan:** docs/testing/2026-03-18-full-service-interactive-qa-testplan.md
**Executor:** Claude Code + Playwright-MCP
**Duration:** ~45 minutes (ongoing)

---

## Results Summary

| Phase | Name | Status | Details |
|-------|------|--------|---------|
| 0 | Environment Discovery | **PASS** | 6 services, dev_mode=true, 4 healthy models, 8 providers |
| 1 | Auth & Login | **PASS** | Invalid creds rejected, valid login redirects, token works |
| 2 | Project Setup | **PASS** | QA-Test-Project created, workspace_path=/tmp/qa-test-project |
| 3 | Dashboard KPIs | **PASS** | 7 KPIs numeric, 5 chart tabs interactive, API verified |
| 4 | Cost Dashboard | **PASS** | 4 KPIs, Cost by Project chart, 5 API endpoints 200 |
| 5 | File Operations | **PASS** | Create file via API, tree updates, Monaco editor opens |
| 6 | Git Operations | **PASS** | Branch=main, dirty=true (after file create), API confirmed |
| 7 | Roadmap & Milestones | **PASS** | Create roadmap + milestone + feature, API data matches |
| 8 | Feature Map | **PASS** | Kanban view, drag handles, feature cards with status |
| 9 | Goals | **PASS** | CRUD works (API+UI), z-index bug fixed (8f14aea) |
| 10 | Model Management | **PASS** | 650+ models discovered, 8 providers, Discover button works |
| 11 | LLM Key Management | **PASS** | API Keys section in Settings, Create Key form present |
| 12 | Chat UI Navigation | **PASS** | Input, Send, Conversation list, context suggestions |
| 13 | Simple Message | **PASS** | Send + receive + model badge (lm_studio/qwen3) |
| 14 | Streaming | **PASS** | Progressive text, typing indicator, Agentic badge |
| 15 | Agentic Tool-Use | **PARTIAL** | Agent dispatched, HITL appeared but timed out |
| 16 | HITL Permissions | **PASS** | PermissionRequestCard: tool, command, countdown, Allow/Deny/Always |
| 25 | Mode Management | **PASS** | 24 built-in modes, full detail cards (tools, denied, scenario, autonomy) |
| 35 | Notifications | **PARTIAL** | Badge "(1)" appeared, NotificationCenter not opened |
| 36 | Settings & Preferences | **PASS** | 9 sections: General, Shortcuts, VCS, Providers, LLM Proxy, Subscriptions, API Keys, Users, Dev Tools |

## Statistics

- **Phases tested:** 20/42
- **PASS:** 18
- **PARTIAL:** 2 (Phase 15: tool timeout, Phase 35: badge only)
- **FAIL:** 0
- **SKIP:** 0
- **Remaining:** 22 phases (17, 18-24, 26-34, 37-41)

## Bugs Found

| Bug | Severity | Status | Commit |
|-----|----------|--------|--------|
| GoalsPanel Create button overlapped by Chat Send | LOW | FIXED | 8f14aea |
| Note: Playwright pointer interception issue, not real user bug | INFO | - | - |

## Notable Observations

1. **Onboarding Stepper** auto-updates when goals/roadmap created (Repo -> Stack -> Goals -> Roadmap -> Agent)
2. **Chat Suggestions** context-sensitive: change per active panel (Goals/Roadmap/Feature Map/Files)
3. **GitHub Copilot** subscription connected, Claude Max disconnected
4. **24 built-in modes** with comprehensive role definitions
5. **HITL Permission** UI works perfectly with tool name, command args, countdown timer
6. **LM Studio** integration healthy (qwen3-30b-a3b responding)

## Environment

- Backend: Go, APP_ENV=development, port 8080
- Frontend: Vite 7.3.1, port 3000
- Worker: Python, CODEFORGE_ROUTING_ENABLED=false
- Docker: postgres (healthy), nats (healthy), litellm (healthy)
- Model: lm_studio/qwen/qwen3-30b-a3b (local, Alibaba)
- Playwright: Docker container via host.docker.internal:3000
