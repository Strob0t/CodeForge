# CodeForge — Marktanalyse & Recherche

> Stand: 2026-02-16

## Projektvision

Containerisierter Service mit Web-GUI zur Orchestrierung von AI-Coding-Agents.
Kernfunktionen:
1. Projekt-Dashboard (SVN/Git/GitHub/GitLab/lokal)
2. Roadmap/Feature-Map-Management (im Repo oder im Service)
3. Multi-LLM-Provider-Management (OpenAI, Claude, lokale Models, etc.)
4. Agent-Orchestrierung fuer Code-Arbeit

---

## 1. Direkte Konkurrenten

### BjornMelin/CodeForge AI
- **URL:** https://github.com/BjornMelin/codeforge
- **Beschreibung:** Multi-Agent-Orchestrierung ueber LangGraph mit Dynamic Model Routing (Grok, Claude, Gemini), GraphRAG+ Retrieval (Qdrant + Neo4j), Debate-Framework fuer Architekturentscheidungen.
- **Stack:** LangGraph 0.5.3+, Qdrant, Neo4j, Redis, Docker Compose, Python 3.12+
- **Status:** Phase 1/MVP, 28 Commits
- **Luecken:** Kein Web-GUI fuer Projektmanagement, kein SCM-Integration, kein Roadmap-Feature

### OpenHands (ehemals OpenDevin)
- **URL:** https://github.com/OpenHands/OpenHands
- **Website:** https://openhands.dev/
- **Beschreibung:** Open-Source AI-Driven Development Platform. Web-GUI, CLI, REST-API. Docker/Kubernetes-Deployment. GitHub/GitLab-Integration. Model-agnostisch.
- **Stars:** 65.000+
- **Lizenz:** MIT (Core), Source-available (Enterprise)
- **Staerken:** Naechster Konkurrent zu unserer Vision, grosse Community, Enterprise-Features
- **Luecken:** Kein Roadmap/Feature-Map-Management, kein SVN-Support, kein Multi-Projekt-Dashboard

### Open SWE (LangChain)
- **URL:** https://github.com/langchain-ai/open-swe
- **Beschreibung:** Cloud-basierter async Coding-Agent. Versteht Codebases, plant Loesungen, erstellt PRs automatisch.
- **Staerken:** GitHub-Integration, async Workflows
- **Luecken:** Kein Multi-Provider-LLM-Management, kein Roadmap-Feature, kein Self-Hosting-Fokus

---

## 2. AI Coding Agents (Partial Overlap)

### SWE-agent
- **URL:** https://github.com/SWE-agent/SWE-agent
- **Beschreibung:** Princeton/Stanford. LLMs loesen autonom GitHub Issues. State-of-the-art auf SWE-bench. Mini-SWE-Agent erreicht 65% auf SWE-bench verified in 100 Zeilen Python.
- **Relevanz:** Potentieller Agent-Backend-Kandidat

### Devika
- **URL:** https://github.com/stitionai/devika
- **Beschreibung:** Open-Source Devin-Alternative. Web UI, Multi-LLM (Claude, GPT, Ollama), AI-Planning, Web-Browsing, Multi-Language Code Generation.
- **Status:** Experimentell, schwierigeres Setup
- **Relevanz:** Aehnliche UI-Idee, aber kein Projektmanagement

### Aider
- **URL:** https://aider.chat / https://github.com/paul-gauthier/aider
- **Beschreibung:** Terminal-basierter AI Pair-Programmer. Git-nativ, Multi-Model-Support.
- **Staerken:** Sehr ausgereift fuer Terminal-Workflows, starke Git-Integration
- **Luecken:** Kein Web-GUI, kein Projektmanagement

### Cline
- **URL:** https://cline.bot
- **Beschreibung:** VS Code Extension. Analysiert Codebases, erstellt/editiert Files, fuehrt Befehle aus. MCP-Support.
- **Staerken:** Reviewable Diffs, Enterprise-Privacy, Custom Workflows via MCP
- **Luecken:** An VS Code gebunden, kein Standalone-Service

---

## 3. Orchestrierungs-Frameworks

### LangGraph
- **URL:** https://github.com/langchain-ai/langgraph
- **Stars:** ~24.700 | **Lizenz:** MIT | **Version:** 1.0.8 (stable)
- **Beschreibung:** Graph-basierte Agent-Orchestrierung von LangChain. StateGraph mit Pregel-Runtime (Bulk Synchronous Parallel). Channels/Reducers fuer State-Management. 6 Streaming-Modi. Production-grade Checkpointing (Postgres, SQLite).
- **Kernkonzepte:** StateGraph, Nodes (Funktionen), Edges (fest/konditional), Channels (LastValue/Topic/BinaryOperator), Pregel-Runtime, Checkpointing, interrupt() fuer HITL
- **Staerken:**
  - Durable Agent Execution mit Crash Recovery und Time-Travel
  - `interrupt()` fuer dynamisches Human-in-the-Loop an beliebiger Stelle
  - 6 Streaming-Modi (values, updates, messages, custom, tasks, debug)
  - PostgresSaver/PostgresStore fuer Production
  - Multi-Agent-Patterns: Supervisor, Swarm, Scatter-Gather
  - Functional API (`@entrypoint`/`@task`) fuer einfache Workflows
- **Schwaechen:**
  - `langchain-core` als Hard-Dependency (~20 transitive Packages)
  - Pregel-Modell mit Channels/Supersteps hat steile Lernkurve
  - Node-Restart bei Interrupt-Resume (gesamter Node wird neu ausgefuehrt)
  - Distributed Runtime nur ueber LangGraph Platform (kommerziell)
  - Kein eingebautes Context-Window-Management
- **Relevanz fuer CodeForge:** StateGraph als Orchestrierungsschicht in Python Workers. Checkpointing ueber PostgresSaver. interrupt() fuer Plan→Approve-Workflow. Streaming fuer UI.

### CrewAI
- **URL:** https://github.com/crewAIInc/crewAI
- **Stars:** ~27.000 | **Lizenz:** MIT | **Version:** 0.114+
- **Beschreibung:** Role-based Multi-Agent-Framework. Agents mit Role/Goal/Backstory. Tasks mit expected_output und Guardrails. Zwei Orchestrierungssysteme: Crew (Tasks) und Flow (DAG).
- **Kernkonzepte:** Agent (Role/Goal/Backstory), Task (Description/ExpectedOutput), Crew (Process: sequential/hierarchical), Flow (@start/@listen/@router DAG), Unified Memory (LanceDB), YAML-Config
- **Staerken:**
  - Intuitive Agent-Definition mit Persona-System (Role/Goal/Backstory)
  - YAML-basierte Agent/Task-Konfiguration + Python-Decorators
  - Unified Memory mit Composite Scoring (Semantic + Recency + Importance)
  - LLM Guardrail Agent — ein Agent validiert den Output eines anderen
  - Flow-System mit @start/@listen/@router fuer DAG-Workflows
  - Event-Bus mit 60+ Event-Typen fuer Observability
  - Human Feedback Provider Protocol (erweiterbar: Web, Slack, Email)
  - @tool Decorator fuer saubere Tool-Definition
  - MCP-Integration nativ
- **Schwaechen:**
  - Kein echter Parallelismus in Crew (nur ueber Flow)
  - Zwei ueberlappende Orchestrierungssysteme (Crew + Flow)
  - ChromaDB + LanceDB beide als Dependencies (redundant)
  - Memory braucht LLM (gpt-4o-mini) + Embedder fuer Basis-Operationen
  - Single-Process, kein Message Queue, keine REST-API
  - Consensual Process nie implementiert
- **Relevanz fuer CodeForge:** YAML-Config-Pattern, Composite Memory Scoring, LLM Guardrail, Event-Bus, Human Feedback Provider Protocol.

### AutoGen (Microsoft)
- **URL:** https://github.com/microsoft/autogen
- **Stars:** ~42.000 | **Lizenz:** MIT | **Version:** 0.7.5 (v0.4+ Architektur)
- **Beschreibung:** Actor-Model-basiertes Multi-Agent-Framework. Saubere Schichtung: autogen-core (Runtime) → autogen-agentchat (Teams) → autogen-ext (Extensions). Distributed Runtime ueber gRPC. Python + .NET.
- **Kernkonzepte:** Agent (Protocol), AgentId (type/key), AgentRuntime (Message Routing), Teams (RoundRobin/Selector/Swarm/GraphFlow/MagenticOne), ChatCompletionClient, Workbench (Tool-Container), Component System
- **Staerken:**
  - Saubere Package-Struktur: Core → AgentChat → Extensions (a-la-carte Dependencies)
  - GraphFlow mit DiGraphBuilder: DAG + Conditional Edges + Parallel Nodes
  - Workbench: Tool-Container mit shared State und dynamischer Tool-Discovery
  - Termination Conditions composable mit & / | Operatoren (12+ Typen)
  - Context-Window-Strategien: Buffered, TokenLimited, HeadAndTail
  - Component System: Agents/Tools/Teams als JSON serialisierbar
  - MagenticOne Orchestrator: Planning Loop + Stall Detection + Re-Planning
  - HandoffMessage Pattern fuer Agent-Uebergabe
  - SocietyOfMindAgent: Team als Agent wrappen (Nested Orchestrierung)
  - Distributed Runtime ueber gRPC (Cross-Language: Python ↔ .NET)
  - Minimale Core-Dependencies (Pydantic, Protobuf, OpenTelemetry)
- **Schwaechen:**
  - Kein LLM-Routing/Load-Balancing (jeder Provider eigener Client)
  - SingleThreadedAgentRuntime nicht fuer High-Concurrency geeignet
  - UserProxyAgent blockiert gesamtes Team
  - Kein eingebauter Persistent Storage (State als Dict, Caller muss persistieren)
  - Komplexe Abstraktionsschichten (Core vs AgentChat)
  - Memory-System noch jung (ListMemory in Core)
- **Relevanz fuer CodeForge:** Layered Package Structure, GraphFlow, Workbench, Termination Conditions, Component System, MagenticOne Orchestrator, HandoffMessage Pattern.

### MetaGPT
- **URL:** https://github.com/geekan/MetaGPT
- **Stars:** ~50.000 | **Lizenz:** MIT
- **Beschreibung:** "Code = SOP(Team)". Simuliert Software-Development-Teams mit spezialisierten Rollen (ProductManager, Architect, Engineer, QA). Dokument-getriebene Pipeline: PRD → Design → Tasks → Code. Strukturierte Zwischenartefakte reduzieren Halluzination.
- **Kernkonzepte:** Role (Profile/Goal/Actions/Watch), Action (LLM-Call + Processing), Message (Pub-Sub mit cause_by Routing), Environment (Shared Space), Team (Hire + Run), ActionNode (Schema-erzwungene Outputs)
- **Staerken:**
  - Dokument-getriebene SOP Pipeline: PRD → Design → Tasks → Code → Test
  - ActionNode: Schema-Validierung + Review/Revise Cycles auf LLM-Output
  - Experience Pool (@exp_cache): Erfolgreiche Runs cachen und wiederverwenden
  - BM25 Tool-Recommendation: Automatisch relevante Tools auswaehlen
  - Budget-Enforcement (NoMoneyException): Harte Kosten-Limits
  - Mermaid-Diagramm-Generierung als Design-Artefakt
  - Incremental Development Mode (bestehenden Code beruecksichtigen)
  - Multi-Environment (Software, Minecraft, Android, Stanford Town)
  - Per-Action LLM Override (verschiedene Models fuer verschiedene Tasks)
  - Message Compression Strategien (pre-cut, post-cut by token/message)
- **Schwaechen:**
  - ~90 direkte Dependencies (massiver Footprint)
  - Single-Process asyncio, kein distributed Runtime
  - Tension zwischen rigid SOPs und dynamischem RoleZero
  - Memory simplistisch (Message-Liste, optionale Vector-Search)
  - Kosten-Management nur global, nicht pro Role/Action
  - Python 3.9-3.11 only (kein 3.12+)
  - Kein Web-GUI (nur CLI, MGX kommerziell)
- **Relevanz fuer CodeForge:** Document Pipeline, ActionNode/Structured Output, Experience Pool, BM25 Tool-Recommendation, Budget-Enforcement, Incremental Development.

---

## 4. LLM-Routing & Multi-Provider

### LiteLLM
- **URL:** https://github.com/BerriAI/litellm
- **Stars:** ~22.000 | **Lizenz:** MIT | **Version:** 1.81+
- **Beschreibung:** Universeller LLM-Proxy (Python). Einheitliche OpenAI-kompatible API (`litellm.completion()`) fuer 127+ Provider. Production-grade Proxy Server (FastAPI + Postgres + Redis). Router mit 6 Routing-Strategien (latency/cost/usage/least-busy/shuffle/tag-based). Fallback-Ketten mit Cooldown. Budget-Management pro Key/Team/User. 42+ Observability-Integrations (Langfuse, Prometheus, Datadog, etc.). Caching (Redis, Semantic, In-Memory).
- **Kernkonzepte:** `litellm.completion()` (unifizierter Einstiegspunkt), `Router` (Load Balancing + Fallbacks), Proxy Server (FastAPI, Port 4000), `model_list` (YAML-Config), `BaseConfig` (Provider-Abstraktion), `CustomStreamWrapper` (Streaming), Callbacks/Hooks
- **Staerken:**
  - 127+ Provider nativ (OpenAI, Anthropic, Gemini, Bedrock, Ollama, vLLM, LM Studio, etc.)
  - OpenAI-kompatible REST-API — jeder Client der OpenAI spricht, spricht automatisch LiteLLM
  - Router: 6 Routing-Strategien + Fallback-Ketten + Cooldown bei Provider-Ausfaellen
  - Budget-Management: Per-Key, Per-Team, Per-User, Per-Provider Limits
  - Docker-Image vorhanden (`docker.litellm.ai/berriai/litellm:main-stable`)
  - Structured Output cross-provider (Schema als Tool-Call bei Providern ohne native Unterstuetzung)
  - 42+ Observability-Integrations (Prometheus, Langfuse, Datadog, etc.)
  - Caching: In-Memory, Redis, Semantic (Qdrant), S3, GCS
  - Model-Aliase: Logische Namen zu echten Provider-Models mappen
  - Per-Call Cost Tracking mit umfassender Preis-Datenbank (36.000+ Zeilen JSON)
- **Schwaechen:**
  - Monolithische Codebasis (6.500+ Dateien, `main.py` 7.400 Zeilen mit if/elif-Kette)
  - Python-only — muss als separater Service laufen, nicht in Go einbettbar
  - Proxy braucht Postgres fuer persistentes Spend-Tracking und Key-Management
  - Memory-Footprint: 200-500MB+ RAM im Proxy-Modus
  - Hohe Aenderungsrate (haeufige Releases, gelegentlich Breaking Changes)
  - Error-Mapping ueber 127 Provider nicht immer perfekt
  - Kein eingebautes Prompt-Management
- **Relevanz fuer CodeForge:** **Zentrale Architekturentscheidung — LiteLLM Proxy als Docker-Sidecar.** Kein eigenes LLM-Provider-Interface noetig. Go Core spricht OpenAI-Format gegen LiteLLM. Python Workers nutzen `litellm.completion()` direkt. Routing, Fallbacks, Budgets, Cost-Tracking delegiert an LiteLLM.

### OpenRouter
- **URL:** https://openrouter.ai
- **Stars:** n/a (Cloud SaaS) | **Modelle:** 300+ | **Provider:** 60+
- **Beschreibung:** Cloud-hosted Unified API Gateway fuer LLMs. Single Endpoint (`/api/v1/chat/completions`) routet zu 300+ Models ueber 60+ Provider. ~30 Billionen Tokens/Monat, 5M+ User. OpenAI-kompatible API. Auto-Router (NotDiamond AI) fuer intelligente Modell-Auswahl. BYOK (Bring Your Own Keys) Support.
- **Kernkonzepte:** Provider Routing (Preis/Latenz/Throughput-Sortierung), Model Fallbacks (Cross-Model), Auto Router (AI-basierte Modell-Auswahl), Model Variants (:free, :nitro, :thinking, :online), Credits-System, BYOK
- **Staerken:**
  - 300+ Models, 60+ Provider ueber einen Endpoint
  - OpenAI-kompatible API (1-Zeilen-Integration via Base-URL-Aenderung)
  - Auto-Router: AI waehlt optimales Model je nach Prompt
  - Provider-Routing: Sortierung nach Preis/Latenz/Throughput, Whitelist/Blacklist
  - Model Fallbacks: Cross-Model-Fallback-Ketten
  - Zero Data Retention (ZDR) Option
  - Rankings/Leaderboard basierend auf echten Nutzungsdaten
  - Message Transforms: Intelligente Prompt-Kompression bei Context-Overflow
- **Schwaechen:**
  - **Cloud-only — kein Self-Hosting** (Kernproblem fuer CodeForge)
  - ~5.5% Platform-Fee auf alle Ausgaben
  - Keine lokalen Models (Ollama, LM Studio nicht unterstuetzt)
  - Privacy-Abhaengigkeit: Alle Prompts transitieren OpenRouter-Infrastruktur
  - Credits verfallen nach 1 Jahr
  - Kein Volume-Discount
- **Relevanz fuer CodeForge:** Als optionaler Provider **hinter** LiteLLM. LiteLLM hat native OpenRouter-Unterstuetzung (`openrouter/<model-id>`). Nutzer die OpenRouter bevorzugen, konfigurieren es als LiteLLM-Deployment. CodeForge baut keine eigene OpenRouter-Integration.

### Claude Code Router
- **URL:** https://github.com/musistudio/claude-code-router
- **Stars:** ~27.800 | **Lizenz:** MIT | **Version:** 2.0.0 (npm)
- **Beschreibung:** Lokaler Proxy speziell fuer Claude Code CLI. Setzt `ANTHROPIC_BASE_URL` auf localhost, faengt alle Requests ab, routet zu konfigurierten Providern (OpenAI, Gemini, DeepSeek, Groq, etc.). Transformer-Chain-Architektur fuer Request/Response-Transformation. Scenario-basiertes Routing (default/background/think/longContext/webSearch).
- **Kernkonzepte:** Transformer Chain (composable Request/Response-Transformers), Scenario-based Routing, Provider Config (JSON5), Preset System (Export/Import/Share), Custom Router Functions (JS-Module), Token-Threshold Routing, Subagent Routing
- **Staerken:**
  - **Scenario-basiertes Routing** — verschiedene Models fuer verschiedene Task-Typen:
    - `default`: Allgemeine Coding-Tasks
    - `background`: Nicht-interaktive Tasks (guenstigere Models)
    - `think`: Reasoning-intensive Operationen (Thinking-Models)
    - `longContext`: Automatisch bei Tokens > Threshold (grosse Context-Windows)
    - `webSearch`: Web-Such-faehige Models
  - Transformer-Chain: Composable, geordnete Transformers fuer Provider-Normalisierung
  - 22 Transformer-Adapter (Provider-spezifisch + Feature-Adapter)
  - Preset-System: Routing-Konfigurationen exportieren/teilen
  - Token-basiertes Routing: Auto-Switch zu Long-Context-Models ab Threshold
  - Custom Router Functions: User-definierte Routing-Logik als JS-Module
  - React-basierte Config-UI (`ccr ui`)
- **Schwaechen:**
  - Claude-Code-spezifisch (funktioniert nur als Proxy fuer Anthropic CLI)
  - 714 offene Issues (Stabilitaetsprobleme)
  - Keine formellen GitHub-Releases
  - Kein Load Balancing, keine Fallback-Ketten
  - Kein Cost-Tracking, kein Budget-Management
  - Single-User, keine Multi-Tenancy
  - Node.js-only, fragile Streaming-Transformation
- **Relevanz fuer CodeForge:** **Scenario-basiertes Routing** ist das Kernkonzept. CodeForge uebernimmt die Idee (default/background/think/longContext/review/plan), implementiert sie aber ueber LiteLLM's Tag-based Routing statt als eigenen Proxy. Token-Threshold-Routing und Preset-System ebenfalls als Features geplant.

### OpenCode CLI
- **URL:** https://github.com/opencode-ai/opencode (archiviert) → https://opencode.ai (TypeScript-Rewrite)
- **Stars:** n/a (archiviert) | **Lizenz:** MIT
- **Beschreibung:** Open-Source Terminal AI-Agent. Original in Go (archiviert, Nachfolger: Crush by Charm + OpenCode by Anomaly/SST). 7 Go-Clients decken 12 Provider ab ueber OpenAI-kompatibles Base-URL-Pattern. GitHub Copilot Token-Exchange. Lokale Model Auto-Discovery. TypeScript-Rewrite (opencode.ai) nutzt Vercel AI SDK + Models.dev fuer 75+ Provider.
- **Kernkonzepte:** OpenAI-compatible Base URL Pattern (1 SDK fuer viele Provider), GitHub Copilot Token Exchange, Local Model Auto-Discovery (/v1/models), Provider Priority Chain, Context File Interoperability (CLAUDE.md, .cursorrules, copilot-instructions.md), Per-Model Pricing Data
- **Staerken:**
  - Zeigt: Meisten Provider sind OpenAI-kompatibel — Base-URL reicht
  - GitHub Copilot als Free Provider (Token aus `~/.config/github-copilot/hosts.json`)
  - Auto-Discovery: Lokale Models via `/v1/models` Endpoint erkennen
  - Provider Priority Chain (Copilot > Anthropic > OpenAI > Gemini > ...)
  - Context File Interoperability (liest CLAUDE.md, .cursorrules etc.)
  - Per-Session Cost-Tracking mit hardcoded Pricing
- **Schwaechen:**
  - Go-Codebase archiviert (Split in Crush + OpenCode TypeScript)
  - Hardcoded Model-Katalog (jedes neue Model braucht Code-Aenderung)
  - Kein Multi-Provider-Routing (ein Provider pro Agent)
  - Kein Load Balancing, keine Fallbacks
  - Single-Agent-Architektur
  - Kein Web-GUI
- **Relevanz fuer CodeForge:** **Drei Patterns uebernommen:** (1) GitHub Copilot Token-Exchange als Provider in Go Core, (2) Auto-Discovery fuer lokale Models (Ollama/LM Studio `/v1/models` abfragen), (3) Provider Priority Chain fuer intelligente Defaults ohne Konfiguration.

### Architekturentscheidung: Kein eigenes LLM-Interface

Die Analyse aller vier Tools fuehrt zu einer klaren Entscheidung:

**CodeForge baut KEIN eigenes LLM-Provider-Interface.** LiteLLM deckt 127+ Provider ab, inklusive Routing, Fallbacks, Cost-Tracking, Budgets, Streaming und Tool-Calling. Das selber zu bauen wuerde Monate kosten und waere permanent hinter LiteLLM's Provider-Coverage.

#### Was CodeForge NICHT baut
- Keinen eigenen LLM-Provider-Proxy
- Keine eigene Provider-Abstraktion in Go oder Python
- Kein eigenes Cost-Tracking auf Token-Ebene (LiteLLM macht das)
- Keine eigene Fallback/Retry-Logik fuer LLM-Calls

#### Was CodeForge BAUT
| Komponente | Schicht | Beschreibung |
|---|---|---|
| **LiteLLM Config Manager** | Go Core | Generiert/aktualisiert LiteLLM Proxy YAML-Config |
| **User-Key-Mapping** | Go Core | Mappt CodeForge-User auf LiteLLM Virtual Keys |
| **Scenario-Routing** | Go Core | Mappt Task-Typen auf LiteLLM-Tags (default/background/think/longContext/review/plan) |
| **Cost Dashboard** | Frontend | Zieht Spend-Daten aus LiteLLM API (`/spend/logs`, `/global/spend/per_team`) |
| **Local Model Discovery** | Go Core | Auto-Discovery via Ollama/LM Studio `/v1/models` Endpoint |
| **Copilot Token Exchange** | Go Core | GitHub Copilot Token aus lokaler Config austauschen |

#### Integrations-Architektur

```
TypeScript Frontend (SolidJS)
        |
        v  REST / WebSocket
Go Core Service
        |
        v  HTTP (OpenAI-kompatible API)
LiteLLM Proxy (Docker Sidecar, Port 4000)
        |
        v  Provider APIs
OpenAI / Anthropic / Ollama / Bedrock / OpenRouter / etc.
```

Go Core und Python Workers sprechen beide mit LiteLLM ueber die standard OpenAI-API. Go Core nutzt den OpenAI Go SDK oder raw HTTP. Python Workers nutzen `litellm.completion()` direkt.

#### Scenario-basiertes Routing (inspiriert von Claude Code Router)

| Scenario | Beschreibung | Beispiel-Routing |
|---|---|---|
| `default` | Allgemeine Coding-Tasks | Claude Sonnet / GPT-4o |
| `background` | Nicht-interaktive Tasks, Batch | GPT-4o-mini / DeepSeek |
| `think` | Reasoning-intensive Aufgaben | Claude Opus / o3 |
| `longContext` | Input > Token-Threshold | Gemini Pro (1M Context) |
| `review` | Code Review, Quality Check | Claude Sonnet |
| `plan` | Architektur, Design | Claude Opus |

Implementiert ueber LiteLLM's Tag-based Routing: Go Core setzt `metadata.tags` im Request, LiteLLM routet zum passenden Model-Deployment.

---

## 5. Spec-Driven Development, Roadmap-Tools & Projektmanagement

### 5.1 Spec-Driven Development (SDD) Tools

#### OpenSpec
- **URL:** https://github.com/Fission-AI/OpenSpec
- **Website:** https://openspec.dev/
- **Stars:** ~24.000 | **Lizenz:** MIT | **CLI:** `openspec` (npm)
- **Beschreibung:** Brownfield SDD Framework. Specs leben im Repo (`openspec/specs/` als Source of Truth, `openspec/changes/` fuer Delta-Proposals). CLI-basiert, kein Web-GUI. Integration mit 22+ AI-Tools (Claude Code, Cursor, Windsurf, Cline, Aider, etc.).
- **Kernkonzepte:**
  - **Spec-Verzeichnis:** `openspec/specs/` — YAML/Markdown Requirements, API-Specs, Datenmodelle
  - **Change-Proposals:** `openspec/changes/` — Delta-Format: ADDED/MODIFIED/REMOVED Requirements
  - **CLI-Kommandos:** `openspec init`, `openspec review`, `openspec apply`, `openspec status`
  - **JSON-Output:** `--json` Flag fuer maschinell verarbeitbare CLI-Ausgabe
  - **Detection:** `openspec/` Verzeichnis im Repo-Root
- **Staerken:**
  - Brownfield-faehig (bestehende Projekte ohne grosse Umstellung)
  - Delta-Spec-Format elegant fuer Change-Management
  - Agent-agnostisch (funktioniert mit jedem AI-Tool)
  - CLI mit `--json` fuer programmatische Integration
  - Wachsende Community (~24k Stars)
- **Schwaechen:**
  - Kein Web-GUI
  - Kein bidirektionaler Sync zu PM-Tools
  - Kein Task-Tracking (nur Specs, keine Issues/Tasks)
- **Relevanz fuer CodeForge:** Primaeres SDD-Format. Auto-Detection ueber `openspec/` Verzeichnis. Delta-Spec-Format fuer Change-Proposals uebernommen. CLI-Integration fuer `openspec review`/`apply` als Agent-Tool.

#### GitHub Spec Kit
- **URL:** https://github.com/spec-kit/spec-kit
- **Stars:** ~16.000+ | **Lizenz:** MIT
- **Beschreibung:** Greenfield SDD Framework. `.specify/` Ordner mit `spec.md`/`plan.md`/`tasks/`. Feature-Nummerierung. Agent-agnostisch.
- **Kernkonzepte:**
  - **Spec-Verzeichnis:** `.specify/` mit `spec.md`, `plan.md`, `tasks/*.md`
  - **Pipeline:** Spec → Plan → Tasks (strukturierte Zerlegung)
  - **Feature-Nummerierung:** Jedes Feature bekommt eine eindeutige Nummer
  - **Detection:** `.specify/` Verzeichnis im Repo-Root
- **Staerken:**
  - Intuitive Markdown-basierte Struktur
  - Klare Pipeline: Spec → Plan → Tasks
  - Agent-agnostisch
  - Gute Community-Adoption
- **Schwaechen:**
  - Kein Brownfield-Support (nur fuer neue Features)
  - Kein Delta-Format (vollstaendige Spec-Rewrites)
  - Kein CLI fuer maschinelle Verarbeitung
- **Relevanz fuer CodeForge:** Zweites SDD-Format. Auto-Detection ueber `.specify/`. Spec→Plan→Tasks Pipeline als Inspiration fuer Dokument-Pipeline.

#### Autospec
- **URL:** https://github.com/Autospec-AI/autospec
- **Beschreibung:** YAML-first SDD. `specs/spec.yaml`, `specs/plan.yaml`, `specs/tasks.yaml`. Ideal fuer programmatische Integration.
- **Kernkonzepte:**
  - **Spec-Verzeichnis:** `specs/` mit YAML-Dateien
  - **YAML-Format:** Strukturiert, maschinenlesbar, versionierbar
  - **Detection:** `specs/spec.yaml` im Repo
- **Staerken:**
  - YAML-Format ideal fuer Go/Python-Parsing (keine Markdown-Ambiguitaet)
  - Programmatisch erzeugbar und validierbar
  - Klare Schema-Definition
- **Schwaechen:**
  - Kleinere Community als OpenSpec/Spec Kit
  - YAML weniger menschenlesbar als Markdown fuer lange Texte
- **Relevanz fuer CodeForge:** Drittes SDD-Format. YAML ideal fuer maschinelle Verarbeitung. Auto-Detection ueber `specs/spec.yaml`.

#### Weitere SDD-Tools

| Tool | Ansatz | Detection-Marker | Relevanz |
|---|---|---|---|
| **BMAD-METHOD** | Multi-Agent Design mit spezialisierten Personas | `bmad/` Verzeichnis | Persona-Pattern interessant fuer Agent-Spezialisierung |
| **Amazon Kiro** | Spec-basierte IDE (Commercial) | `.kiro/` Verzeichnis | Zeigt kommerziellen Trend zu SDD |
| **cc-sdd** | Claude Code SDD Extension | `.sdd/` Verzeichnis | Leichtgewichtige SDD-Variante |

### 5.2 Projektmanagement-Tools (Open Source)

#### Plane.so
- **URL:** https://plane.so / https://github.com/makeplane/plane
- **Stars:** ~45.600 | **Lizenz:** AGPL-3.0
- **Beschreibung:** Modernes Open-Source-PM mit AI-Features, Roadmaps, Wiki, GitHub/GitLab-Sync. REST API v1 mit 180+ Endpoints. Python SDK verfuegbar.
- **Kernkonzepte:**
  - **Hierarchie:** Workspace → Initiative → Project → Epic → Work Item (Issue) + Cycles + Modules
  - **API:** REST `/api/v1/`, cursor-basierte Pagination, Field Selection (`expand`, `fields`), CRUD fuer alle Entitaeten
  - **SDK:** `plane-sdk` (Python) — typisierte Clients fuer alle Ressourcen
  - **Webhooks:** Events fuer Issues, Cycles, Modules mit HMAC-SHA256 Signierung
  - **MCP Server:** Offizielle MCP-Integration fuer AI-Assistenten
  - **Labels:** Label-triggered Automation und Sync
- **Staerken:**
  - Staerkstes Open-Source-PM-Tool (45.6k Stars, aktive Entwicklung)
  - Umfassende REST API mit 180+ Endpoints (gut dokumentiert)
  - Python SDK fuer einfache Integration
  - Bidirektionaler GitHub/GitLab-Sync (Issues, Labels, Comments)
  - Initiative/Epic/WorkItem-Hierarchie bildet Roadmap ab
  - Cursor-basierte Pagination (skaliert gut)
  - Field Selection und Expansion (reduziert API-Traffic)
  - Webhooks mit HMAC-SHA256 fuer sichere Integration
  - MCP Server fuer AI-Integration
  - AGPL-3.0 (Self-Hosted moeglich)
- **Schwaechen:**
  - Kein SVN-Support
  - Kein AI-Coding-Agent-Integration
  - API nur REST (kein GraphQL)
  - AGPL-3.0 erfordert Vorsicht bei Integration
- **Relevanz fuer CodeForge:** Primaerer PM-Platform-Adapter. REST API als Sync-Ziel. Uebernommene Patterns: Initiative/Epic/WorkItem-Hierarchie, cursor-basierte Pagination, Field Selection, Webhook HMAC-SHA256, Label-triggered Sync.

#### OpenProject
- **URL:** https://www.openproject.org/ / https://github.com/opf/openproject
- **Stars:** ~14.400 | **Lizenz:** GPL v3
- **Beschreibung:** Enterprise PM mit Gantt-Charts, Version Boards, Roadmaps. HAL+JSON API v3 mit HATEOAS. OAuth 2.0. GitHub/GitLab Webhook-Integration. Ruby on Rails.
- **Kernkonzepte:**
  - **API:** HAL+JSON API v3 mit HATEOAS (Self-describing Links), 50+ Endpoint-Familien
  - **Auth:** OAuth 2.0 (PKCE) + API Keys
  - **Work Packages:** Zentrale Entitaet mit 20+ Typen (Task, Bug, Feature, Epic, etc.)
  - **Versions:** Versions-basierte Roadmaps (Releases/Milestones)
  - **Gantt:** Interaktive Gantt-Charts mit Abhaengigkeiten
  - **SCM-Integration:** GitHub/GitLab Webhooks (Pull Requests → Work Packages)
  - **Schema-Endpoints:** `/api/v3/work_packages/schema` fuer dynamische Formulare
  - **Form-Endpoints:** `/api/v3/work_packages/form` fuer Validierung vor Submit
  - **Notification Reasons:** Granulare Benachrichtigungs-Gruende (mentioned, assigned, responsible, watched)
- **Staerken:**
  - Enterprise-ready (14+ Jahre Entwicklung, ISO 27001)
  - Gantt-Charts und Version Boards (echte Roadmap-Features)
  - HATEOAS-API (Self-Describing, Discovery ueber Links)
  - Optimistic Locking via `lockVersion` (Konflikt-Erkennung)
  - Schema/Form-Endpoints fuer dynamische UIs
  - GitHub/GitLab Webhook-Integration (PR → Work Package Link)
  - Notification Reasons (granulare Benachrichtigungen)
- **Schwaechen:**
  - HAL+JSON zu komplex fuer Go Core (Heavy Parsing)
  - Ruby on Rails Monolith (schwierig zu embedden)
  - GPL v3 (restriktiver als MIT/AGPL)
  - API-Dokumentation teilweise veraltet
- **Relevanz fuer CodeForge:** Zweiter PM-Platform-Adapter. Uebernommene Patterns: Optimistic Locking (lockVersion), Schema-Endpoints fuer dynamische Forms, Notification Reasons. HAL+JSON explizit NICHT uebernommen (zu komplex fuer Go Core, normales JSON REST reicht).

#### Ploi Roadmap
- **URL:** https://github.com/ploi/roadmap
- **Lizenz:** MIT | **Stack:** Laravel (PHP)
- **Beschreibung:** Einfaches Open-Source Roadmap-Tool mit innovativem `/ai` Endpoint fuer Machine-Readable Data.
- **Kernkonzepte:**
  - **`/ai` Endpoint:** Stellt Roadmap-Daten in JSON/YAML/Markdown bereit — speziell fuer LLM-Konsum
  - **Roadmap-Board:** Kanban-artige Darstellung (Under Review → Planned → In Progress → Live)
  - **Webhooks:** Einfache Event-Benachrichtigungen
  - **Voting:** User-Voting auf Roadmap-Items
- **Staerken:**
  - `/ai` Endpoint — innovatives Pattern fuer Machine-Readable Roadmap-Daten
  - Einfache, klare Architektur
  - MIT-Lizenz (maximal permissiv)
  - Voting-Feature fuer Community-Feedback
- **Schwaechen:**
  - Minimaler Funktionsumfang (kein Gantt, kein Epic/Story, keine Hierarchie)
  - Laravel/PHP-Stack (nicht direkt integrierbar)
  - Keine API fuer CRUD (nur Read via `/ai`)
- **Relevanz fuer CodeForge:** `/ai` Endpoint-Pattern uebernommen. CodeForge stellt eigenen `/api/v1/roadmap/ai` Endpoint bereit, der Roadmap-Daten fuer LLM-Konsum in JSON/YAML/Markdown formatiert.

#### Weitere PM-Tools

| Tool | Stars | Lizenz | Kernfeature | Relevanz fuer CodeForge |
|---|---|---|---|---|
| **Huly** | ~22.000 | EPL-2.0 | All-in-one PM, bidirektionaler GitHub-Sync | Sync-Architektur als Referenz |
| **Linear** | Closed Source | Commercial | GraphQL API, MCP Server, beste DX | GraphQL-Pattern, MCP als Integration |
| **Leantime** | ~5.000 | AGPL-3.0 | Lean PM, Strategy → Portfolio → Project | Strategie-Layer-Konzept |
| **Roadmapper** | ~500 | MIT | Golang-basiertes Roadmap-Tool | Go-Implementierung als Referenz |

### 5.3 SCM-basiertes Projektmanagement

Viele Teams nutzen die eingebauten PM-Features ihrer SCM-Plattform (GitHub Issues, GitLab Issues/Boards). CodeForge muss diese erkennen und integrieren.

| Plattform | PM-Features | API | Detection |
|---|---|---|---|
| **GitHub** | Issues, Projects (v2), Milestones, Labels | REST v3 + GraphQL v4 | Remote-URL `github.com` |
| **GitLab** | Issues, Boards, Milestones, Epics, Roadmaps | REST v4 + GraphQL | Remote-URL `gitlab.com` oder Self-Hosted |
| **Gitea/Forgejo** | Issues, Labels, Milestones, Projects | REST (GitHub-kompatibel) | Remote-URL + `/api/v1/version` |

**Gitea/Forgejo-Insight:** Die GitHub-kompatible API bedeutet, dass CodeForge's GitHub-Adapter mit minimalen Aenderungen auch fuer Gitea/Forgejo funktioniert. Empfehlung: Gitea als dritten SCM-Adapter implementieren, basierend auf dem GitHub-Adapter.

### 5.4 Repo-basiertes Projektmanagement

Einige Tools speichern PM-Artefakte direkt im Repository. CodeForge sollte diese erkennen und integrieren.

| Tool | Detection-Marker | Format | Beschreibung |
|---|---|---|---|
| **Markdown Projects (mdp)** | `.mdp/` Verzeichnis | Markdown | Projekte als Markdown-Dateien im Repo |
| **Backlog.md** | `backlog/` Verzeichnis | Markdown | Backlog als Markdown-Dateien |
| **git-bug** | Eingebettet in Git-Objekte | Git Objects | Bug-Tracking direkt in Git (kein externer Service) |
| **Tasks.md** | `TASKS.md` Datei | Markdown | Einfache Task-Liste als Markdown |
| **markdown-plan** | `PLAN.md` / `ROADMAP.md` | Markdown | Roadmap als Markdown |

### 5.5 ADR/RFC Tools

Architectural Decision Records und RFCs sind in vielen Projekten vorhanden und relevant fuer CodeForge's Planungsfunktionen.

| Detection-Marker | Tool/Konvention | Beschreibung |
|---|---|---|
| `docs/adr/` | ADR Tools (adr-tools, log4brains) | Architectural Decision Records |
| `docs/decisions/` | Alternative ADR-Konvention | Entscheidungs-Dokumentation |
| `docs/rfcs/` oder `rfcs/` | RFC-Prozess | Request for Comments |

CodeForge erkennt diese Verzeichnisse und kann ADRs/RFCs in der Roadmap-Ansicht anzeigen und referenzieren.

### 5.6 Feature-Flag-Tools

Feature-Flags beeinflussen die Roadmap-Sicht (welche Features sind aktiv, im Rollout, etc.).

| Tool | Stars | Lizenz | API | Relevanz |
|---|---|---|---|---|
| **Unleash** | ~12.000 | Apache-2.0 | REST API | Feature-Flag-State in Roadmap integrieren |
| **OpenFeature** | Standard | Apache-2.0 | SDK-Standard | Vendor-neutrales Feature-Flag-Interface |
| **Flagsmith** | ~5.000 | BSD-3 | REST + SDK | Feature-Flags + Remote Config |
| **FeatBit** | ~2.000 | MIT | REST API | Self-Hosted Feature-Flags |
| **GrowthBook** | ~7.000 | MIT | REST + SDK | Feature-Flags + A/B Testing |

Feature-Flag-Integration ist ein Phase-3-Feature. CodeForge kann ueber die REST APIs der Tools den aktuellen Feature-Status abfragen und in der Roadmap-Ansicht anzeigen.

### 5.7 Auto-Detection Architektur

CodeForge erkennt automatisch, welche Spec-, PM- und Roadmap-Tools in einem Projekt verwendet werden, und bietet passende Integration an.

#### Drei-Tier Detection

```
Tier 1: Spec-Driven Detectors (Repo-Dateien scannen)
  ├── openspec/           → OpenSpec
  ├── .specify/           → GitHub Spec Kit
  ├── specs/spec.yaml     → Autospec
  ├── .bmad/              → BMAD-METHOD
  ├── .kiro/              → Amazon Kiro
  ├── .sdd/               → cc-sdd
  ├── docs/adr/           → ADR Tools
  ├── docs/rfcs/          → RFC-Prozess
  ├── .mdp/               → Markdown Projects
  ├── backlog/            → Backlog.md
  ├── TASKS.md            → Tasks.md
  └── ROADMAP.md          → markdown-plan

Tier 2: Platform Detectors (API-basierte Erkennung)
  ├── Remote-URL Analyse  → GitHub / GitLab / Gitea / Forgejo
  ├── API-Probe           → Plane.so / OpenProject / Huly / Linear
  └── Webhook-Config      → Bestehende Webhook-Setups erkennen

Tier 3: File-Based Detectors (einfache Marker)
  ├── .github/            → GitHub Actions, Issue Templates
  ├── .gitlab-ci.yml      → GitLab CI
  ├── CHANGELOG.md        → Changelog-Management
  └── .env / .env.example → Environment-Konfiguration
```

#### Detection-Ablauf

```
1. Repo wird zu CodeForge hinzugefuegt
     ↓
2. Go Core scannt Repo-Root auf Detection-Marker (Tier 1 + 3)
     ↓
3. Go Core analysiert Remote-URL und probt Platform-APIs (Tier 2)
     ↓
4. Erkannte Tools werden dem User angezeigt:
   "Erkannt: OpenSpec, GitHub Issues, ADRs"
     ↓
5. User konfiguriert Integration:
   - Welche Tools aktiv verfolgt werden
   - Sync-Richtung (Import / Export / Bidirektional)
   - Sync-Frequenz (Webhook / Poll / Manuell)
     ↓
6. Go Core richtet Sync ein (Webhooks registrieren, Poll-Jobs schedulen)
```

### 5.8 Architekturentscheidungen: Roadmap/PM-Integration

| Entscheidung | Begruendung |
|---|---|
| **Kein eigenes PM-Tool** | Plane, OpenProject, GitHub Issues existieren. CodeForge synchronisiert, statt neu zu erfinden. |
| **Repo-basierte Specs als First-Class** | OpenSpec, Spec Kit, Autospec leben im Repo — CodeForge behandelt sie als primaere Roadmap-Quelle. |
| **Bidirektionaler Sync** | Aenderungen in CodeForge → PM-Tool und umgekehrt. Konflikt-Resolution via Timestamps + User-Entscheidung. |
| **Provider Registry Pattern** | Gleiche Architektur wie `gitprovider` und `llmprovider` — neue PM-Adapter erfordern nur neues Package + Blank-Import. |
| **Cursor-basierte Pagination** (von Plane) | Skaliert besser als Offset-basiert fuer grosse Datenmengen. Fuer CodeForge's eigene API und PM-Sync. |
| **HAL+JSON NICHT uebernommen** (von OpenProject) | Zu komplex fuer Go Core. Normales JSON REST mit klaren Endpoints reicht. |
| **Label-triggered Sync** (von Plane) | Labels als Trigger fuer automatischen Sync — z.B. Label "codeforge-sync" aktiviert bidirektionale Synchronisierung. |
| **`/ai` Endpoint** (von Ploi Roadmap) | Dedizierter Endpoint, der Roadmap-Daten fuer LLM-Konsum aufbereitet (JSON/YAML/Markdown). |

### 5.9 Uebernommene Patterns

#### Von Plane.so

| Pattern | Umsetzung in CodeForge |
|---|---|
| Initiative/Epic/WorkItem-Hierarchie | CodeForge Roadmap-Modell: Milestone → Feature → Task |
| Cursor-basierte Pagination | Standard-Pagination fuer CodeForge API und PM-Sync |
| Field Selection (`expand`, `fields`) | API-Responses konfigurierbar — nur benoetigte Felder |
| Webhook HMAC-SHA256 | Sichere Webhook-Verifizierung fuer eingehende Events |
| Label-triggered Sync | Label "codeforge-sync" aktiviert bidirektionale Sync |
| MCP Server | CodeForge stellt eigenen MCP Server bereit fuer AI-Integration |

#### Von OpenProject

| Pattern | Umsetzung in CodeForge |
|---|---|
| Optimistic Locking (lockVersion) | Konflikt-Erkennung bei gleichzeitigen Aenderungen |
| Schema-Endpoints | `/api/v1/{resource}/schema` fuer dynamische Form-Generierung in der GUI |
| Form-Endpoints | `/api/v1/{resource}/form` fuer Validierung vor Submit |
| Notification Reasons | Granulare Benachrichtigungen (mentioned, assigned, responsible, watching) |

#### Von OpenSpec

| Pattern | Umsetzung in CodeForge |
|---|---|
| Delta-Spec-Format | Change-Proposals als ADDED/MODIFIED/REMOVED Deltas |
| Change-Proposal-Workflow | Spec-Aenderungen durchlaufen Review → Apply Pipeline |
| `--json` CLI-Output | Agent-Tools erhalten maschinenlesbare Outputs |

#### Von GitHub Spec Kit

| Pattern | Umsetzung in CodeForge |
|---|---|
| Spec → Plan → Tasks Pipeline | Strukturierte Zerlegung in Dokument-Pipeline |
| Feature-Nummerierung | Eindeutige Feature-IDs fuer Referenzierung |

#### Von Autospec

| Pattern | Umsetzung in CodeForge |
|---|---|
| YAML-first Artefakte | Specs/Plans/Tasks in YAML (maschinenlesbar, validierbar) |

#### Von Ploi Roadmap

| Pattern | Umsetzung in CodeForge |
|---|---|
| `/ai` Endpoint | `/api/v1/roadmap/ai` — Roadmap-Daten fuer LLM-Konsum (JSON/YAML/Markdown) |

#### Explizit NICHT uebernommen

| Konzept | Grund |
|---|---|
| HAL+JSON / HATEOAS (OpenProject) | Zu komplex fuer Go Core, normales JSON REST reicht |
| GraphQL API (Linear) | REST als Primaer-API, GraphQL eventuell spaeter |
| Eigenes PM-Tool | Sync mit bestehenden Tools statt Neuentwicklung |
| Ruby on Rails Patterns (OpenProject) | Go Core hat eigene Architektur (Hexagonal) |
| Plane's AGPL-Code | Keine Code-Uebernahme, nur API-Integration und Pattern-Inspiration |

---

## 6. Self-Hosted LLM Plattformen

### Dify
- **URL:** https://github.com/langgenius/dify
- **Website:** https://dify.ai
- **Beschreibung:** Open-Source LLM App Development. Visual Workflow Builder, RAG, Agent Capabilities, LLMOps. Docker Compose Deployment.
- **Stars:** ~129.000
- **Relevanz:** Bestes Beispiel fuer Self-Hosted LLM-Plattform mit UI. UI/UX-Inspiration.

### AnythingLLM
- **URL:** https://github.com/Mintplex-Labs/anything-llm
- **Beschreibung:** All-in-one Desktop & Docker AI Application. RAG, AI Agents, No-code Agent Builder, MCP.
- **Relevanz:** Zeigt wie All-in-one Docker AI aussehen kann

### Open WebUI
- **URL:** https://github.com/open-webui/open-webui
- **Beschreibung:** Self-hosted AI Interface. Ollama + OpenAI-kompatibel. Docker/Kubernetes.
- **Relevanz:** UI-Patterns fuer LLM-Interaktion

---

## 7. Marktbewertung

| Bereich                              | Marktstatus         | Unsere Chance                          |
|--------------------------------------|---------------------|----------------------------------------|
| AI Coding Agents                     | Ueberfuellt (>20)   | Nicht neu erfinden, integrieren        |
| Multi-LLM-Routing                    | Geloest             | LiteLLM/OpenRouter nutzen              |
| Self-hosted Web-GUI Agent            | 1-2 Player          | OpenHands dominiert                    |
| Spec-Driven Development              | Fragmentiert (6+)   | Auto-Detection + Multi-Format-Support  |
| PM-Tool-Integration                  | Viele Silos         | Bidirektionaler Sync als Aggregator    |
| Roadmap + Agent + Multi-Projekt      | **Keine Loesung**   | **Hauptdifferenzierung**               |
| SVN-Support bei AI-Agents            | **Null**            | **Alleinstellungsmerkmal**             |
| Auto-Detection + Adaptive Integration| **Existiert nicht** | **Technisches Alleinstellungsmerkmal** |
| Integrierte Plattform (alle 4 Saeulen) | **Existiert nicht** | **Kernangebot von CodeForge**         |

---

## 8. Strategische Empfehlungen

### Baue auf bestehenden Bausteinen:
- **LLM-Routing:** LiteLLM als Proxy-Layer (statt eigenes Routing)
- **Agent-Backends:** Integration von Aider, OpenHands, SWE-agent als austauschbare Backends
- **Spec-Formate:** Multi-Format-Support (OpenSpec, Spec Kit, Autospec) mit Auto-Detection
- **PM-Integration:** Sync mit Plane, OpenProject, GitHub/GitLab Issues (statt eigenes PM-Tool)

### Differenziere durch Integration:
- Zentrales Dashboard fuer mehrere Projekte (Git, GitHub, GitLab, SVN, Gitea/Forgejo)
- Visuelles Roadmap-Management mit bidirektionalem Sync zu Repo-Specs UND PM-Tools
- **Auto-Detection:** Automatische Erkennung aller Spec/PM/Roadmap-Tools im Repo
- **Adaptive Integration:** Je nach erkannten Tools passende Sync-Strategie anbieten
- LLM-Provider-Management mit Task-basiertem Routing
- Agent-Orchestrierung die verschiedene Coding-Agents koordiniert

### Vermeide:
- Eigenen LLM-Proxy von Grund auf bauen (LiteLLM existiert)
- Eigenen Coding-Agent von Grund auf bauen (integriere bestehende)
- Eigenes PM-Tool von Grund auf bauen (sync mit Plane/OpenProject/GitHub Issues)
- Feature-Krieg mit OpenHands auf deren Kerngebiet (einzelne Issues loesen)
- HAL+JSON/HATEOAS — zu komplex fuer Go, normales JSON REST reicht

---

## 9. Framework-Vergleich: LangGraph vs CrewAI vs AutoGen vs MetaGPT

### Architektur-Vergleich

| Dimension | LangGraph | CrewAI | AutoGen | MetaGPT |
|---|---|---|---|---|
| **Metapher** | State Machine / Graph | Crew mit Tasks | Actor Model / Pub-Sub | Software-Firma mit SOPs |
| **State-Modell** | Zentral (Shared State Dict) | Im Crew-Kontext | Verteilt (jeder Agent eigener State) | Environment + Memory + Documents |
| **Kommunikation** | State-Mutation (Dict Updates) | Tool-basierte Delegation | Message Passing (typed) | Pub-Sub mit cause_by Routing |
| **Agent-Identitaet** | Keine (Nodes = Funktionen) | Role/Goal/Backstory | Erste Klasse (AgentId, Lifecycle) | Role/Profile/Actions/Watch |
| **Orchestrierung** | Graph-Topologie (Edges) | Process (seq/hierarchical) | Teams (5 Typen) | SOP Pipeline + TeamLeader |
| **Persistenz** | Built-in Checkpointing | Flow Persistence | State Save/Load (manuell) | Serialization + Git Repo |
| **Distributed** | Nur Platform ($) | Nein | Ja (gRPC nativ) | Nein |
| **LangChain-Kopplung** | Ja (langchain-core) | Nein (entfernt) | Nein (optional) | Nein |
| **Dependencies** | Mittel (~20 transitiv) | Schwer (ChromaDB+LanceDB+OTel) | Minimal (Core), modular (Ext) | Sehr schwer (~90 direkt) |

### Feature-Vergleich

| Feature | LangGraph | CrewAI | AutoGen | MetaGPT |
|---|---|---|---|---|
| Sequential | Edges | Process.sequential | RoundRobin | SOP Pipeline |
| Hierarchisch | Subgraphs | Process.hierarchical | SelectorGroupChat | TeamLeader Hub |
| DAG/Graph | StateGraph | Flow (@start/@listen) | GraphFlow (DiGraph) | Nein |
| Parallel | Send API | Flow (and_/or_) | GraphFlow (activation) | asyncio.gather |
| Handoff/Swarm | langgraph-swarm | DelegateWorkTool | Swarm + HandoffMessage | publish_message |
| Nested Teams | Subgraph als Node | Crew in Flow | SocietyOfMindAgent | Nein |
| Planning Loop | Custom Nodes | planning=True | MagenticOne | Plan-and-Act Mode |
| Human-in-Loop | interrupt() | human_input + Provider | UserProxyAgent | HumanProvider + AskReview |
| Streaming | 6 Modi | Token + Events | 3 Ebenen (Token/Agent/Team) | LLM-Level |
| Structured Output | Nein (LLM-nativ) | output_json/pydantic | StructuredMessage[T] | ActionNode + Review/Revise |
| Memory (Short) | Checkpointer | In Crew-Kontext | ChatCompletionContext | Message-Liste |
| Memory (Long) | BaseStore (KV+Vector) | Unified (LanceDB) | ChromaDB/Redis/Mem0 | Vector (optional) |
| Tool System | ToolNode (LangChain) | BaseTool + @tool + MCP | Workbench + MCP | ToolRegistry + BM25 |
| Guardrails | RetryPolicy | LLM Guardrail Agent | Termination Conditions | Budget-Enforcement |
| YAML Config | Nein | Agents + Tasks | Component System (JSON) | Nein |
| Event System | Debug Stream | Event-Bus (60+ Types) | Nein | Nein |
| Experience Cache | Nein | Nein | Nein | Experience Pool |
| Document Pipeline | Nein | Nein | Nein | PRD→Design→Code |
| Cost Management | Nein | Nein | Token-basiert | Budget + NoMoneyException |

### Synthese: Was CodeForge uebernimmt

#### Von LangGraph

| Konzept | Umsetzung in CodeForge |
|---|---|
| StateGraph + Checkpointing | Orchestrierungsschicht in Python Workers, PostgresSaver |
| interrupt() fuer HITL | Plan→Approve→Execute Workflow |
| 6 Streaming-Modi | UI-Feedback (Token, State-Updates, Custom Events, Debug) |
| PostgresStore | Long-Term Memory Backend |
| Functional API | Einfachere Workflows via @entrypoint/@task |

#### Von CrewAI

| Konzept | Umsetzung in CodeForge |
|---|---|
| YAML Agent/Task Config | Agent-Spezialisierung als YAML, GUI-konfigurierbar |
| Unified Memory (Composite Scoring) | Recall mit Semantic + Recency + Importance Gewichtung |
| LLM Guardrail Agent | Quality Layer: Agent validiert Agent-Output |
| Event-Bus (60+ Events) | Observability fuer Dashboard, Monitoring, WebSocket |
| Flow DAG (@start/@listen/@router) | Inspiration fuer Workflow-Editor in der GUI |
| Human Feedback Provider Protocol | Erweiterbare HITL-Kanaele (Web-GUI, Slack, Email) |
| @tool Decorator | Saubere Tool-Definition fuer eigene Tools |

#### Von AutoGen

| Konzept | Umsetzung in CodeForge |
|---|---|
| Layered Package Structure | Core → AgentChat → Extensions, saubere Trennung |
| GraphFlow (DiGraphBuilder) | DAG + Conditional Edges + Parallel Nodes fuer Agent-Koordination |
| Workbench (Tool-Container) | Shared State fuer zusammengehoerige Tools, MCP-Integration |
| Termination Conditions (composable) | Flexible Stop-Bedingungen mit & / \| Operatoren |
| Context-Window-Strategien | Buffered, TokenLimited, HeadAndTail (kein Context-Overflow) |
| Component System (deklarativ) | Agents/Tools/Workflows als JSON — essentiell fuer GUI-Editor |
| MagenticOne Orchestrator | Planning Loop + Stall Detection + Re-Planning |
| HandoffMessage Pattern | Agent-Uebergabe zwischen Spezialisten (Aider→OpenHands→SWE-agent) |
| SocietyOfMindAgent | Team als Agent wrappen fuer nested Orchestrierung |

#### Von MetaGPT

| Konzept | Umsetzung in CodeForge |
|---|---|
| Dokument-getriebene Pipeline | PRD→Design→Tasks→Code reduziert Halluzination |
| ActionNode (Schema-Validierung) | Erzwungene Strukturen + Review/Revise Cycles |
| Experience Pool (@exp_cache) | Erfolgreiche Runs cachen, Kosten sparen |
| BM25 Tool-Recommendation | Automatisch relevante Tools auswaehlen |
| Budget-Enforcement | Harte Kosten-Limits pro Task/Projekt/User |
| Mermaid-Generierung | Automatische Architektur-Visualisierung |
| Incremental Development | Bestehenden Code beruecksichtigen bei Generierung |
| Per-Action LLM Override | Verschiedene Models fuer verschiedene Schritte |

#### Explizit NICHT uebernommen

| Konzept | Grund |
|---|---|
| LangChain Message Types | Eigenes Message-Format, LLM via LiteLLM |
| CrewAI's ChromaDB + LanceDB | Zu schwer, PostgresStore + pgvector reicht |
| AutoGen's per-Provider LLM Clients | LiteLLM routet alles einheitlich |
| MetaGPT's 90 Dependencies | Schlanke Workers, nur was gebraucht wird |
| Alle Single-Process Runtimes | Go Core orchestriert, Python Workers fuehren aus |
| LangGraph Platform / CrewAI Enterprise | Self-hosted by Design |
| AutoGen's gRPC Runtime | Go Core uebernimmt Agent-Lifecycle via NATS/Redis |
| MetaGPT's Pydantic-Vererbungsketten | Komposition statt tiefe Vererbung |
