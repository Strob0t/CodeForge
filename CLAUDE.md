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
- Detaillierte Beschreibung: docs/architecture.md

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
