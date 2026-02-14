# CodeForge — Projektkontext

## Was ist CodeForge?

Containerisierter Service zur Orchestrierung von AI-Coding-Agents mit Web-GUI.

### Vier Kernsaeulen:
1. **Projekt-Dashboard** — Verwaltung mehrerer Repos (Git, GitHub, GitLab, SVN, lokal)
2. **Roadmap/Feature-Map** — Visuelles Management, kompatibel mit OpenSpec, bidirektionaler Sync zu Repo-Specs
3. **Multi-LLM-Provider** — OpenAI, Claude, lokale Models (Ollama/LM Studio), Routing via LiteLLM
4. **Agent-Orchestrierung** — Koordination verschiedener Coding-Agents (Aider, OpenHands, SWE-agent, etc.)

## Architektur

Drei-Schichten Hybrid-Stack:

```
TypeScript Frontend (SolidJS)
        |
        v  REST / WebSocket
Go Core Service (HTTP, WebSocket, Agent Lifecycle, Repo-Verwaltung, Scheduling)
        |
        v  Message Queue (NATS/Redis)
Python AI Workers (LLM Calls, Agent Execution, LiteLLM, LangGraph)
```

## Tech Stack

| Schicht        | Sprache    | Zweck                                    |
|----------------|------------|------------------------------------------|
| Frontend       | TypeScript | Web-GUI                                  |
| Core Service   | Go 1.23    | HTTP/WS Server, Scheduling, Repo-Mgmt   |
| AI Workers     | Python 3.12| LLM-Integration, Agent-Ausfuehrung      |
| Infrastructure | Docker     | Containerisierung, Docker-in-Docker      |

## Tooling

- **Python:** Poetry, Ruff (Linting + Formatting), Pytest
- **Go:** golangci-lint, gofmt, goimports
- **TypeScript:** ESLint, Prettier
- **Alle:** pre-commit hooks (.pre-commit.yaml), Docker Compose

## Marktpositionierung

Die spezifische Kombination aus Projekt-Dashboard + Roadmap + Multi-LLM + Agent-Orchestrierung existiert nicht.
Naechster Konkurrent: OpenHands (kein Roadmap, kein Multi-Projekt-Dashboard, kein SVN).
Detaillierte Analyse: docs/research/market-analysis.md

## Software-Architektur

- **Hexagonal Architecture (Ports & Adapters)** fuer den Go Core
- **Provider Registry Pattern** fuer Open-Source-Erweiterbarkeit (Self-Registering via `init()`)
- **Capabilities** statt Pflicht-Implementierung — jeder Provider deklariert, was er kann
- **Compliance-Tests** pro Interface — neue Adapter erben automatisch die Test-Suite
- **LLM Capability Levels** — Workers ergaenzen fehlende Faehigkeiten je nach LLM:
  - Vollwertige Agents (Claude Code, Aider): nur Orchestrierung
  - API mit Tools (OpenAI, Gemini): + Context Layer (GraphRAG) + Routing
  - Reine Completion (Ollama, lokal): + alles (Context, Tools, Quality Layer)
- **Worker-Module:** Context (GraphRAG), Quality (Debate/Reviewer/Sampler),
  Routing, Safety, Execution, History, Hooks, Trajectory
- **Agent Execution Modes:** Sandbox (isolierter Container), Mount (direkter Dateizugriff), Hybrid
- **Command Safety Evaluator:** Shell-Befehle pruefen, Tool-Blocklists (YAML)
- **Agent-Workflow:** Plan → Approve → Execute → Review → Deliver (konfigurierbar)
- **YAML-basierte Tool-Bundles:** Deklarative Tool-Definitionen, kein Code noetig
- **History Processors:** Context-Window-Optimierung als Pipeline
- **Hook-System:** Observer-Pattern fuer Agent/Environment-Lifecycle
- **Trajectory Recording:** Aufzeichnung, Replay, Inspector, Audit-Trail
- **Kosten-Management:** Budget-Limits pro Task/Projekt/User, Auto-Tracking
- **Jinja2-Prompt-Templates:** Prompts in separaten Dateien, nicht im Code
- **KeyBERT Keyword-Extraction:** Semantische Keywords fuer besseres Retrieval
- **Real-time State via WebSocket:** Live-Updates fuer Agent-Status, Logs, Kosten
- **Agent-Spezialisierung:** YAML-konfigurierbare Sub-Agents (offener Punkt, spaeter)
- **Frontend:** SolidJS + Tailwind CSS
- **Framework-Insights (LangGraph, CrewAI, AutoGen, MetaGPT):**
  - Composite Memory Scoring (Semantic + Recency + Importance)
  - Context-Window-Strategien (Buffered, TokenLimited, HeadAndTail)
  - Experience Pool (@exp_cache) fuer Caching erfolgreicher Runs
  - Tool-Recommendation via BM25 (automatische Tool-Auswahl)
  - Workbench (Tool-Container mit shared State, MCP-Integration)
  - LLM Guardrail Agent (Agent validiert Agent-Output)
  - Structured Output / ActionNode (Schema-Validierung + Review/Revise)
  - Event-Bus fuer Observability (Agent/Task/System Events → WebSocket)
  - GraphFlow / DAG-Orchestrierung (Conditional Edges, Parallel Nodes, Cycles)
  - Composable Termination Conditions (MaxSteps | Budget | Timeout)
  - Component System (Agents/Tools/Workflows als JSON serialisierbar, GUI-Editor)
  - Dokument-Pipeline PRD→Design→Tasks→Code (reduziert Halluzination)
  - MagenticOne Planning Loop (Stall Detection + Re-Planning)
  - HandoffMessage Pattern (Agent-Uebergabe zwischen Spezialisten)
  - Human Feedback Provider Protocol (Web-GUI, Slack, Email erweiterbar)
- **Coding-Agent-Insights (Cline, Devika):**
  - Plan/Act Mode Pattern (separate LLM-Configs pro Phase, User-Toggle)
  - Shadow-Git Checkpoints (isoliertes Git-Repo fuer Rollback)
  - Ask/Say Approval Pattern (granulare Permissions pro Tool-Kategorie)
  - MCP als Standard-Extensibility-Protokoll fuer Tools
  - .clinerules-aehnliche Projekt-Konfiguration (YAML-basiert)
  - Auto-Compact Context Management (Zusammenfassung bei ~80% Window)
  - Diff-basiertes File Review (Side-by-Side vor Approval)
  - Sub-Agent-Architektur (Planner/Researcher/Coder-Trennung)
  - Agent State Visualization (Internal Monologue, Steps, Browser, Terminal)
  - LLM-gesteuerter Web Crawler (Page Content → LLM → Action Loop)
  - Stateless Agent Design (State im Core, nicht in Agents)
- **Coding-Agent-Insights (OpenHands, SWE-agent):**
  - Event-Sourcing Architecture (EventStream als zentrale Abstraktion)
  - Workspace-Abstraktion (Local/Docker/Remote, Self-Healing Containers)
  - AgentHub mit spezialisierten Agents (CodeAct, Browsing, Delegator, Microagents)
  - Microagents: YAML+Markdown-basierte trigger-gesteuerte Spezial-Agents
  - Skills-System (wiederverwendbare Python-Snippets, automatisch in Prompt)
  - Risk Management mit LLMSecurityAnalyzer (InvariantAnalyzer)
  - V0→V1 SDK Migration Pattern (AgentSkills als MCP-Server)
  - RouterLLM fuer lokale Routing-Entscheidung (OpenRouter als Fallback)
  - ACI (Agent-Computer Interface): Fuer LLMs optimierte Shell-Befehle
  - Tool-Bundles (YAML): Deklarative, austauschbare Tool-Definitionen
  - History Processors: Pipeline fuer Context-Window-Optimierung
  - SWE-ReX Sandbox: Docker-basierte Remote-Execution
  - Mini-SWE-Agent Pattern: 100 Zeilen Python, 74% SWE-bench
  - ToolFilterConfig: Blocklist + Conditional Blocking fuer Command Safety
- **Erweiterte Konkurrenzanalyse (12 neue Tools):**
  - Codel (Go+React, Docker-Sandbox, AGPL-3.0) — Architektur-Referenz
  - CLI Agent Orchestrator (AWS, Supervisor/Worker, tmux/MCP) — naechster Konkurrent
  - Goose (Rust, MCP-native, 30k+ Stars, Apache 2.0) — Backend-Kandidat
  - OpenCode (Go, Client/Server, LSP, MIT) — Backend-Kandidat
  - Plandex (Go, Planning-First, Diff-Sandbox, MIT) — Backend-Kandidat
  - Roo Code (Modes-System, Cloud Agents, Apache 2.0) — Pattern-Referenz
  - Codex CLI (OpenAI, Multimodal, GitHub Action, Apache 2.0) — Backend-Kandidat
  - SERA (Ai2, Open Model Weights, $400 Training, Apache 2.0) — Self-Hosted-Modell
  - bolt.diy (19k Stars, 19+ Provider, MIT) — Multi-LLM-Referenz
  - AutoForge (Two-Agent, Test-First, Multi-Session) — Workflow-Pattern
  - Dyad (Local-First, Apache 2.0) — UX-Referenz
  - AutoCodeRover (AST-aware, GPL-3.0, $0.70/Task) — Nischen-Agent
- **Roadmap/Feature-Map Auto-Detection & Adaptive Integration:**
  - **Kein eigenes PM-Tool** — Sync mit bestehenden Tools (Plane, OpenProject, GitHub/GitLab Issues)
  - **Auto-Detection:** Drei-Tier-Erkennung (Repo-Dateien → Platform-APIs → File-Marker)
  - **Multi-Format SDD-Support:** OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`)
  - **Provider Registry:** `specprovider` (Repo-Specs) + `pmprovider` (PM-Plattformen), gleiche Architektur wie Git/LLM
  - **Bidirektionaler Sync:** CodeForge ↔ PM-Tool ↔ Repo-Specs, Webhook/Poll/Manuell
  - **Uebernommene Patterns:** Plane (Cursor-Pagination, HMAC-SHA256, Label-Sync), OpenProject (Optimistic Locking, Schema-Endpoints), OpenSpec (Delta-Spec-Format), Ploi Roadmap (`/ai` Endpoint)
  - **Gitea/Forgejo:** GitHub-Adapter funktioniert mit minimalen Aenderungen (kompatible API)
  - Detaillierte Analyse: docs/research/market-analysis.md Abschnitt 5
- **LLM-Integration (LiteLLM, OpenRouter, Claude Code Router, OpenCode CLI):**
  - **Kein eigenes LLM-Provider-Interface** — LiteLLM Proxy als Docker-Sidecar (Port 4000)
  - Go Core + Python Workers sprechen OpenAI-kompatible API gegen LiteLLM
  - Scenario-basiertes Routing via LiteLLM Tags (default/background/think/longContext/review/plan)
  - OpenRouter als optionaler Provider hinter LiteLLM
  - GitHub Copilot Token-Exchange als Provider (Go Core)
  - Local Model Auto-Discovery (Ollama/LM Studio `/v1/models`)
  - LiteLLM Config Manager, User-Key-Mapping, Cost Dashboard als Eigenentwicklung
- Detaillierte Beschreibung: docs/architecture.md
- Framework-Vergleich: docs/research/market-analysis.md

## Strategische Prinzipien

- Bestehende Bausteine nutzen (LiteLLM, OpenSpec, Aider/OpenHands als Backends)
- Nicht das Rad neu erfinden bei Einzelkomponenten
- Differenzierung durch Integration aller vier Saeulen
- Performance-Fokus: Go fuer Core, Python nur fuer AI-spezifische Arbeit

## Git-Workflow

- **Commits nur auf `staging`** — niemals direkt auf `main`, es sei denn der User gibt explizit die Anweisung dazu
- **Branch-Strategie:** Entwicklung auf `staging`, Merge nach `main` nur auf Anweisung
- **Vor jedem Commit — Checkliste:**
  1. `pre-commit run --all-files` ausfuehren und Fehler beheben
  2. Betroffene Dokumentation aktualisieren:
     - `docs/architecture.md` — bei Architektur- oder Strukturaenderungen
     - `docs/dev-setup.md` — bei neuen Verzeichnissen, Ports, Tooling, Environment-Variablen
     - `docs/tech-stack.md` — bei neuen Dependencies, Sprach-/Tool-Versionen
     - `docs/project-status.md` — erledigte Punkte abhaken, neue Punkte eintragen
     - `CLAUDE.md` — bei Aenderungen an Kernsaeulen, Architektur, Workflow-Regeln
