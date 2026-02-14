# CodeForge — Projektstatus

> Letztes Update: 2026-02-16

## Phase 0: Projekt-Setup (aktuell)

### Erledigt

- [x] Marktrecherche durchgefuehrt (docs/research/market-analysis.md)
  - 20+ bestehende Projekte analysiert
  - Marktluecke identifiziert: Keine integrierte Loesung fuer Projekt-Dashboard + Roadmap + Multi-LLM + Agent-Orchestrierung
  - SVN-Support als Alleinstellungsmerkmal bestaetigt
- [x] Architekturentscheidung: Go + TypeScript + Python (Drei-Schichten-Hybrid)
- [x] Devcontainer konfiguriert (Go 1.23, Python 3.12, Node.js 22, Docker-in-Docker)
- [x] Linting/Formatting fuer alle drei Sprachen (Ruff, golangci-lint, ESLint/Prettier)
- [x] Pre-commit Hooks (.pre-commit.yaml)
- [x] Python Paketmanagement mit Poetry (pyproject.toml)
- [x] Docker Compose fuer Dev-Services (docs-mcp, playwright-mcp)
- [x] MCP Server Integration (.mcp.json)
- [x] .gitignore
- [x] CLAUDE.md (Projektkontext fuer AI-Assistenten)
- [x] Dokumentation (docs/)
- [x] Software-Architektur definiert: Hexagonal Architecture + Provider Registry Pattern
- [x] LLM Capability Levels und Worker-Module definiert (GraphRAG, Debate, Routing)
- [x] Agent Execution Modes definiert (Sandbox, Mount, Hybrid)
- [x] Agent-Workflow definiert (Plan → Approve → Execute → Review → Deliver)
- [x] Command Safety Evaluator und Tool-Provisioning konzipiert
- [x] Quality Layer erweitert: Action Sampling, RetryAgent + Reviewer, Debate (3 Stufen)
- [x] YAML-basierte Tool-Bundles, History Processors, Hook-System, Trajectory Recording
- [x] Kosten-Management konzipiert (Budget-Limits pro Task/Projekt/User)
- [x] Konkurrenzanalyse vertieft: BjornMelin/codeforge, Open SWE, SWE-agent, Devika
- [x] Jinja2-Prompt-Templates, KeyBERT, Real-time WebSocket State konzipiert
- [x] Frontend-Framework gewaehlt: SolidJS + Tailwind CSS
- [x] Git-Workflow mit Commit-Checkliste (pre-commit + Doku-Pflege)
- [x] Orchestrierungs-Frameworks analysiert: LangGraph, CrewAI, AutoGen, MetaGPT
  - Detaillierter Feature-Vergleich und Architektur-Mapping
  - Adoptierte Patterns identifiziert und dokumentiert
- [x] Framework-Insights in Architektur integriert:
  - Composite Memory Scoring (Semantic + Recency + Importance)
  - Context-Window-Strategien (Buffered, TokenLimited, HeadAndTail)
  - Experience Pool (@exp_cache) fuer Caching erfolgreicher Runs
  - Tool-Recommendation via BM25, Workbench (Tool-Container)
  - LLM Guardrail Agent, Structured Output / ActionNode
  - Event-Bus fuer Observability, GraphFlow / DAG-Orchestrierung
  - Composable Termination Conditions, Component System (deklarativ)
  - Dokument-Pipeline PRD→Design→Tasks→Code
  - MagenticOne Planning Loop (Stall Detection + Re-Planning)
  - HandoffMessage Pattern, Human Feedback Provider Protocol
- [x] LLM-Routing & Multi-Provider analysiert: LiteLLM, OpenRouter, Claude Code Router, OpenCode CLI
  - LiteLLM: 127+ Provider, Proxy Server, Router (6 Strategien), Budget-Management, 42+ Observability
  - OpenRouter: 300+ Models, Cloud-only, ~5.5% Fee → als Provider hinter LiteLLM
  - Claude Code Router: Scenario-basiertes Routing (default/background/think/longContext)
  - OpenCode CLI: OpenAI-compatible Base URL Pattern, Copilot Token Exchange, Auto-Discovery
- [x] Architekturentscheidung: Kein eigenes LLM-Interface, LiteLLM Proxy als Docker-Sidecar
  - Go Core und Python Workers sprechen OpenAI-kompatible API gegen LiteLLM (Port 4000)
  - Scenario-basiertes Routing via LiteLLM Tag-based Routing
  - Eigenentwicklung: Config Manager, User-Key-Mapping, Scenario Router, Cost Dashboard
  - Local Model Discovery (Ollama/LM Studio), Copilot Token Exchange
- [x] Roadmap/Spec/PM-Tools analysiert: OpenSpec, Spec Kit, Autospec, Plane.so, OpenProject, Ploi Roadmap
  - 6+ SDD-Tools analysiert (OpenSpec, GitHub Spec Kit, Autospec, BMAD-METHOD, Amazon Kiro, cc-sdd)
  - 4+ PM-Tools analysiert (Plane.so, OpenProject, Ploi Roadmap, Huly, Linear)
  - Repo-basierte PM-Tools kartiert (Markdown Projects, Backlog.md, git-bug, Tasks.md)
  - ADR/RFC-Tools und Feature-Flag-Tools als Erweiterungen identifiziert
  - Gitea/Forgejo als GitHub-kompatible SCM-Alternative identifiziert
- [x] Auto-Detection Architektur konzipiert: Drei-Tier Detection (Repo → Platform → File)
  - Spec-Driven Detectors: OpenSpec, Spec Kit, Autospec, ADR/RFC
  - Platform Detectors: GitHub, GitLab, Plane.so, OpenProject
  - File-Based Detectors: ROADMAP.md, TASKS.md, CHANGELOG.md
- [x] Provider Registry erweitert: specprovider + pmprovider (gleiche Architektur wie Git/LLM)
- [x] Architekturentscheidung: Kein eigenes PM-Tool, bidirektionaler Sync mit bestehenden
  - Uebernommene Patterns: Cursor-Pagination, HMAC-SHA256, Label-Sync (Plane), Optimistic Locking, Schema-Endpoints (OpenProject), Delta-Spec-Format (OpenSpec), `/ai` Endpoint (Ploi Roadmap)
  - Explizit NICHT uebernommen: HAL+JSON/HATEOAS, GraphQL, eigenes PM-Tool

### Offen

- [ ] Devcontainer erstmalig bauen und testen
- [ ] Go-Modul initialisieren (go mod init)
- [ ] Grundlegende Projektstruktur anlegen (Verzeichnisse fuer Go, Python, Frontend)
- [ ] Agent-Spezialisierung detailliert ausarbeiten (YAML-Configs, GUI-Workflow-Editor)

## Phase 1: Foundation (naechster Schritt)

- [ ] Go Core Service Grundgeruest (HTTP Server, Router, Health-Endpoint)
- [ ] Python Worker Grundgeruest (Queue Consumer, Health-Check)
- [ ] Frontend Grundgeruest (Leere App mit Routing)
- [ ] Message Queue Setup (NATS oder Redis)
- [ ] Datenbank-Entscheidung und Setup
- [ ] CI/CD Pipeline (GitHub Actions)

## Phase 2: MVP Features

- [ ] Projekt-Management (Repo hinzufuegen/entfernen, Status anzeigen)
- [ ] Git-Integration (Clone, Pull, Branch, Diff)
- [ ] LLM-Provider-Verwaltung (API Keys, Model-Auswahl)
- [ ] Einfache Agent-Ausfuehrung (einzelner Task an einzelnen Agent)
- [ ] Basic Web-GUI fuer alle oben genannten Features

## Phase 3: Erweiterte Features

- [ ] Roadmap/Feature-Map Editor (Auto-Detection, Multi-Format SDD, bidirektionaler PM-Sync)
- [ ] OpenSpec/Spec Kit/Autospec-Integration
- [ ] SVN-Integration
- [ ] Multi-Agent-Orchestrierung
- [ ] GitHub/GitLab Webhook-Integration
- [ ] Kosten-Tracking fuer LLM-Nutzung
- [ ] Multi-Tenancy / Nutzerverwaltung
