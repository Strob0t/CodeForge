# Session Summary — 2026-03-23

**Duration:** Full day session
**Branch:** staging
**Focus:** Autonomous Multi-Language Testplan — Design, Runs, Bug Analysis, Weak Model Adaptation, Knowledge System Integration

---

## Phase 1: Brainstorming & Design

### Recherchen (2 parallele Agents)
- **Benchmark-Landschaft:** 25+ Benchmarks analysiert (SWE-bench, Commit0, DevBench, FeatureBench, Multi-SWE-bench, Terminal-Bench). Kein Benchmark testet Multi-Language-Integration innerhalb eines Projekts.
- **Showcase-Demos:** Codex 3D-Racer, MetaGPT Snake/Flappy Bird, Devin Migrationen, bolt.new SaaS. Kein Konkurrent zeigt Python+TypeScript Integration als Showcase.

### Design-Entscheidungen
- Mode A (Weather Dashboard: Python FastAPI + TypeScript/SolidJS) + Mode D (Freie Wahl)
- Modus-Auswahl per interaktiver Rückfrage (nicht hardcoded)
- Goals im natürlichen Chat-Dialog erarbeiten (Claude Code als Auftraggeber, CodeForge Agent als Entwickler)
- Roadmap: Agent beschreibt Struktur, Claude Code erstellt via UI (Agent hat kein Roadmap-Tool)
- Jeder Eingriff von Claude Code = dokumentierter Bug (INFO/WARNING/CRITICAL)
- 4-Tier-Verifikation: funktional, Qualität, semantisch, Cross-Language

### Artefakte
- `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md` — Design-Spec (2x reviewed, 17 Issues gefixt, APPROVED)
- `docs/plans/2026-03-22-autonomous-multi-language-testplan.md` — Implementation Plan (11 Tasks, 28 Steps, reviewed, APPROVED)
- `docs/testing/autonomous-multi-language-testplan.md` — Executable Runbook (1.267 Zeilen, 11 Phasen, Playwright-MCP Befehle, Decision Trees)

---

## Phase 2: Test Runs (3 Runs mit qwen3-30b)

### Run 1 — Großer Prompt, keine Fixes
- **Score: 57% (16/28)**
- Backend: FastAPI mit wttr.in, Caching, CORS — funktioniert
- Frontend: SolidJS scaffolded aber React-Code geschrieben (useState statt createSignal)
- Tests: `</n` Syntax-Fehler, keine Frontend-Tests, kein Git-Commit
- Goal-Researcher Mode komplett übersprungen
- **8 Bugs dokumentiert**

### Run 2 — Großer Prompt + explizites SolidJS, mit Bug-Fixes
- Bug #2 Fix bestätigt: Agent rief `propose_goal` auf
- LiteLLM Tag-Fix bestätigt
- Gestoppt nach 10 min um Konversations-Strategie zu testen

### Run 3 — Konversations-Strategie (kurze Nachrichten)
- Agent schlug 1 relevantes Goal vor, driftete dann in `create_skill`-Error-Loop
- 1 Datei produziert (README.md), kein Code
- **~5% Score**
- 3 neue Bugs dokumentiert (#9, #10, #11)

---

## Phase 3: Bug-Analyse & Fixes (Worktree 1: `fix/testplan-bugfixes`)

### Recherchen (3 parallele Agents)
- Plan/Act Tool-Filtering: Roo Code, Cline, SWE-agent, LangGraph, CrewAI Patterns analysiert
- Framework-Erkennung: Cline, Cursor, Aider, bolt.new, OpenHands, Claude Code verglichen
- Post-Write-Linting: SWE-agent edit_linting.sh, Aider tree-sitter, PostToolUse Hook Pattern

### Root Causes & Fixes (7 Commits)
| Bug | Root Cause | Fix |
|---|---|---|
| #2 goal_researcher bypass | `PLAN_TOOLS` hardcoded, `propose_goal` fehlte | `extra_plan_tools` Parameter in PlanActController |
| #4+6 React statt SolidJS | `detectStackSummary()` verwarf Frameworks | Gibt jetzt `"typescript (solidjs)"` zurück |
| #5 `</n` Syntax-Fehler | Lokales Modell Serialisierungs-Artefakt | Post-write `ast.parse()` Check in write_file/edit_file |
| #7+8 Keine Tests/Commits | Prompt nicht explizit genug | Coder-Mode Completion-Checklist |

**Tests:** 21 neue Python-Tests, 3 neue Go-Tests

---

## Phase 4: Weak Model Adaptation (Worktree 2: `fix/weak-model-adaptation`)

### Recherchen (4 parallele Agents)
- **Mini-SWE-agent:** 74% SWE-bench mit nur Bash (100 LOC)
- **"Less is More" Paper:** 20-71% bessere Ergebnisse bei Tool-Reduktion 46→19
- **SWE-agent ACI:** Interface-Design wichtiger als Modell-Größe
- **CodeForge Capability-System:** 3 Levels existieren (full/api_with_tools/pure_completion), aber Tool-Filterung nicht implementiert

### 6 Maßnahmen implementiert (6 Commits)
| # | Maßnahme | Impact |
|---|---|---|
| M1 | Tool-Filterung: pure_completion sieht 6 statt 13+ Tools | Verhindert Tool-Verwechslung |
| M2 | Step-by-Step Prompt: 1 Tool pro Turn, verify nach write | Verhindert Abdriften |
| M3 | Context-Limits: 16K/32K/120K nach Capability | Verhindert Context-Rot |
| M4 | Tool-Guidance: create_skill "NICHT für normales Coding" | Verhindert Tool-Halluzination |
| M5 | Error-Counter: NON-RETRYABLE nach 2x gleichem Fehler | Bricht Error-Loops |
| M6 | Sampling: temp=0.7, top_p=0.8, rep_penalty=1.05 für lokale Modelle | Bessere Ausgaben |

**Tests:** 26 neue Tests (18 capability, 8 error-tracker)

---

## Phase 5: Audit-Run Analyse

### Ergebnis: 25% (6/24) — mit M1-M6 aktiv
- Maßnahmen waren aktiv (Worker-Logs bestätigen capability_level=pure_completion, step-by-step injected, context limit 16K)
- Aber: Modell halluziniert APIs (`lru_cache(ttl=)`), kennt SolidJS-Setup nicht, macht keinen Git-Commit

### Schlüsselerkenntnis
CodeForge hat Knowledge Base, Skills, Microagents, Retrieval, Stack Detection — aber **Knowledge Base und Stack Detection sind nicht an den Agent-Prompt angebunden**.

---

## Phase 6: Knowledge Pipeline (Worktree 3: `fix/knowledge-pipeline`)

### Codebase-Analyse
| System | Gebaut? | An Agent angebunden? |
|---|---|---|
| Knowledge Base (CRUD, Indexing) | Ja | **NEIN** — disconnected |
| Microagents (Pattern-Trigger) | Ja | Ja — funktioniert |
| Skills (BM25-Selektion) | Ja | Ja — aber nur 1 Built-in |
| Retrieval/RAG (BM25 + Semantic) | Ja | Ja |
| Stack Detection | Ja | **NEIN** — erkannt aber ignoriert |
| RepoMap (tree-sitter) | Ja | Ja |

### Fixes (4 Commits)
| Task | Was |
|---|---|
| 1 | `EntryKnowledge` Konstante + `GetScopesForProject()` Store-Methode |
| 2 | `fetchKnowledgeBaseEntries()` in Context-Pipeline (`context_optimizer.go`) |
| 3 | Built-in Framework Skills (solidjs, fastapi YAML) |
| 4 | `_inject_framework_skills()` Stack-Detection → Skill-Injection |

### Revert
Tasks 3+4 auf User-Anweisung revertiert — **keine hardcodierten Framework-Spickzettel**. Wissen kommt über das Knowledge-Base-System (User erstellt es, nicht wir im Code).

**Erhalten:** Tasks 1+2 (generische KB→Context Pipeline)

---

## Phase 7: docs-mcp-server Integration

### Analyse
- `arabold/docs-mcp-server` analysiert: 10 MCP-Tools (scrape_docs, search_docs, etc.), Ollama-Embeddings, SSE/HTTP Transport, SQLite+FTS5+sqlite-vec
- Vergleich CodeForge KB vs docs-mcp-server: docs-mcp-server löst Ingestion-Problem (URL-Scraping, 90+ Formate, Versions-Bewusstsein)

### Recherchen (4 parallele Agents)
- **MCP UI Best Practices:** 7 Tools analysiert (Cline, Claude Code/Desktop, VS Code, Cursor, Roo Code, Windsurf, OpenCode). Claude Code: bestes 3-Tier Scoping. VS Code: Enable/Disable separat von Config.
- **CodeForge UI Analyse:** Backend 100% komplett (API, DB, Store, Handlers). Frontend API Client existiert, wird nie aufgerufen. Integration-Punkt: `CompactSettingsPopover.tsx`
- **25+ akademische Papers:** API-Docs +83-220% für seltene APIs. Context-Länge schadet (14-85%). RAG equalisiert Modelle. Modelle <7B profitieren NICHT von RAG.
- **docs-mcp-server Details:** 10 Tools, Ollama Config, SSE auf :6280, Web-UI auf :6281, Persistent Volumes

### Lösungsvorschläge evaluiert
| Prio | Empfehlung | Begründung |
|---|---|---|
| P0 | MCP→Projekt UI | API existiert, fehlt nur Button |
| P1 | Delegate scraping an docs-mcp-server | Schon in Docker, warum nachbauen |
| P2 | Projekt-Level Assignment reicht | YAGNI bei 1-5 Projekten |
| P3 | Parallele Systeme, nicht verknüpfen | Unterschiedliche Stärken |
| P4 | Fester Sidecar | Schon erledigt (docker-compose) |

### Implementierung (Worktree 4: `fix/knowledge-integration`, 3 Commits)
| Task | Was | Dateien |
|---|---|---|
| 1 | Docker: Ollama Embeddings + Web-UI Port | `docker-compose.yml` |
| 2 | MCP Server Projekt-Zuweisung UI | `CompactSettingsPopover.tsx` |
| 3 | Setup-Anleitung | `docs/dev-setup.md` |

---

## Alle Commits (chronologisch)

### Design & Testplan
- `3bb4766` docs(spec): autonomous multi-language project testplan design
- `ce3b5a0` refactor: move specs/plans out of docs/superpowers/
- `51966e4` docs(plans): implementation plan for multi-language autonomous testplan
- `3852180` docs(testing): autonomous multi-language testplan runbook
- `6b9e3cf` docs: add multi-language autonomous testplan to todo tracker

### Run 1 Report
- `f4936eb` docs(testing): multi-language autonomous testplan — Run 1 report

### Testplan Bugfixes (Worktree 1)
- `c2f72e2` fix: plan/act controller accepts mode-specific extra plan tools (Bug #2)
- `69e1f1b` fix: pass mode tools to PlanActController as extra plan tools
- `20284bb` fix: include detected frameworks in agent stack context (Bug #4+6)
- `515fccf` feat: post-write syntax check module for agent file operations
- `5ecff54` feat: integrate post-write syntax check into write_file and edit_file tools
- `3c1c7c3` fix: add testing/git/framework checklist to coder mode prompt (Bug #7, #8)
- `e103931` test: regenerate golden files after coder prompt update

### Run 2+3 Report + Testplan Update
- `d0a916c` docs(testing): update report with Run 2+3 results, cross-run summary
- `a85230d` docs(testing): add conversational strategy and run learnings to testplan

### Weak Model Adaptation (Worktree 2)
- `e5e7eb8` feat: filter tools by capability level — weak models see fewer tools (M1)
- `58c0ccc` feat: step-by-step workflow prompt for weak models (M2)
- `bab8cab` feat: capability-based context limits — 16K for weak models (M3)
- `3e6824a` feat: complete tool guidance for create_skill and propose_goal (M4)
- `5e602a1` feat: per-tool error counter with NON-RETRYABLE blocking (M5)
- `85c20d6` feat: optimized sampling parameters for local models (M6)
- `6cb23bb` docs: weak model adaptation — design spec + implementation plan

### Knowledge Pipeline (Worktree 3)
- `e2e93f8` feat: add EntryKnowledge kind + GetScopesForProject store method
- `2cebcbd` feat: inject knowledge base entries into conversation context pipeline
- `5eabe17` feat: built-in framework skills for SolidJS and FastAPI
- `ecc2907` feat: auto-inject framework skills based on stack detection
- `f066704` revert: remove hardcoded framework skills and mapping

### Knowledge Integration (Worktree 4)
- `165a615` feat: add docs-mcp-server to docker-compose
- `e4fd850` docs: knowledge integration — spec + plan (docs-mcp-server + MCP UI)
- `b04283a` fix: docs-mcp-server Ollama embeddings + Web Dashboard port
- `e998a7d` feat: MCP server project assignment UI in settings popover
- `88a7dee` docs: add docs-mcp-server setup guide

---

## Zahlen

| Metrik | Wert |
|---|---|
| Commits auf staging | ~30 |
| Worktrees erstellt | 4 |
| Test-Runs durchgeführt | 3 (+ 1 Audit-Run aus anderer Session) |
| Neue Tests | ~90 (Python + Go) |
| Neuer Code | ~2.500 Zeilen |
| Recherche-Agents dispatched | ~15 |
| Akademische Papers analysiert | 25+ |
| Competitor Tools analysiert | 10+ (Cline, Cursor, VS Code, Roo Code, Windsurf, OpenCode, Aider, SWE-agent, OpenHands, bolt.diy) |
| Design Specs geschrieben | 4 |
| Implementation Plans geschrieben | 4 |
| Bugs dokumentiert | 11 |
| Bugs gefixt | 8 |

---

## Phase 8: Run 4b (groq/llama-3.1-8b, M1-M6 aktiv)

**Score: 35%** — Infrastruktur validiert, Wissen fehlt.
- 4 Goals vorgeschlagen (propose_goal funktioniert)
- Zero Tool-Verwechslung (M1 Filterung funktioniert)
- 7 Dateien, 20 Steps, kein create_skill-Loop
- Aber: React statt SolidJS, OpenWeatherMap statt wttr.in (Modell kennt beides nicht)

## Phase 9: docs-mcp-server Integration

- Docker-Compose Konfiguration (LM Studio Embeddings)
- 6 Libraries indexiert: solidjs, fastapi, wttr-in, vite, pytest, wttr.in
- Search funktioniert (SolidJS createSignal Docs abrufbar)
- **MCP Enabled Bug gefunden und gefixt** — `conversation_agent.go` setzte `Enabled` nicht im NATS Payload

## Phase 10: Run 4c+4d (Versuch mit docs-mcp-server)

- Run 4c: MCP Server als "disabled" geskippt (Bug), 3 Dateien, 0 MCP-Aufrufe
- Run 4d: Vorbereitet (Backend, Worker, docs-mcp, Projekt mit MCP zugewiesen)
- **Playwright-MCP Session abgelaufen** — Run konnte nicht über UI gestartet werden
- Bereit für nächste Claude Code Session

## Offene Punkte

| # | Was | Status |
|---|---|---|
| 1 | **Run 4d ausführen** — alles vorbereitet, nur Playwright-MCP Session erneuern | BEREIT |
| 2 | docs-mcp search_docs verifizieren im Agent-Context | BEREIT (MCP Enabled Bug gefixt) |
| 3 | KB→Context Pipeline testen (fetchKnowledgeBaseEntries) | Code gemerged, nicht getestet |
| 4 | Test mit Cloud-Modell (Claude/GPT) | Braucht API-Key |
| 5 | Scope-based MCP Resolution | Aufgeschoben (YAGNI) |
| 6 | Post-Write Lint für TypeScript (`tsc --noEmit`) | Offen |

## Vorbereitung für nächste Session

Alles ist bereit für Run 4d:
```bash
# Services laufen (docker compose)
docker compose up -d postgres nats litellm docs-mcp

# IPs auflösen (WSL2)
source scripts/resolve-docker-ips.sh

# Backend starten (mit korrekten IPs!)
DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
  NATS_URL="nats://${NATS_IP}:4222" APP_ENV=development go run ./cmd/codeforge/

# Worker starten
PYTHONPATH=workers NATS_URL="nats://${NATS_IP}:4222" \
  LITELLM_BASE_URL="http://${LITELLM_IP}:4000" ... .venv/bin/python -m codeforge.consumer

# Frontend
cd frontend && npm run dev

# docs-mcp hat 6 Libraries indexiert (persistent in Docker Volume)
# Projekt weather-r4d existiert mit docs-mcp zugewiesen
# Model: /model lm_studio/qwen/qwen3-30b-a3b
# Mode: /mode goal_researcher
```

## Phase 11: Run 4d — LM Studio + docs-mcp-server (10 MCP Tools)

**MCP Enabled Bug: GEFIXT** — 10 Tools von docs-mcp-server gemerged (search_docs, scrape_docs, list_libraries, etc.)

**Ergebnis:**
- Agent verwendete wttr.in (Verbesserung gegenüber Run 4b's OpenWeatherMap)
- Aber: Agent nutzte search_docs NICHT aktiv
- 7 Steps, stalled bei "repeated bash after 2 escape attempts"
- 2 Dateien (backend/main.py + README.md), kein Frontend
- lru_cache(timeout=600) Bug wieder aufgetreten

**Erkenntnis:** 10 zusätzliche MCP-Tools überfordern ein 3B-Active-Parameter Modell. Das Modell nutzt die neuen Tools nicht — es fällt auf seine Trainingsdaten zurück. Mehr Tools = mehr Verwirrung für schwache Modelle (bestätigt "Less is More" Paper).

**Nächste Schritte:**
1. Proaktive Context-Injection: docs-mcp Search-Ergebnisse VOR dem Agent-Loop in den Context packen (statt darauf zu warten dass der Agent search_docs aufruft)
2. Stärkeres Modell: qwen2.5-coder-32b-instruct (32B dense) oder Cloud-Modell
3. MCP-Tool-Filterung: Für pure_completion nur search_docs und list_libraries exponieren, nicht alle 10
