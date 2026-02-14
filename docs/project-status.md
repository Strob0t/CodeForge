# CodeForge — Projektstatus

> Letztes Update: 2026-02-14

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

- [ ] Roadmap/Feature-Map Editor
- [ ] OpenSpec-Kompatibilitaet
- [ ] SVN-Integration
- [ ] Multi-Agent-Orchestrierung
- [ ] GitHub/GitLab Webhook-Integration
- [ ] Kosten-Tracking fuer LLM-Nutzung
- [ ] Multi-Tenancy / Nutzerverwaltung
