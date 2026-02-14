# CodeForge — Marktanalyse & Recherche

> Stand: 2026-02-15

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
- **Beschreibung:** Universeller LLM-Proxy. Policy-basiertes Routing, Team-Auth, Audit-Logging.
- **Relevanz:** Starker Kandidat fuer die Multi-LLM-Layer in CodeForge

### OpenRouter
- **URL:** https://openrouter.ai
- **Beschreibung:** Cloud-Gateway zu 400+ Models. Kein Self-Hosting, aber einfache Integration.
- **Relevanz:** Als optionaler Provider nutzbar

### Claude Code Router
- **URL:** https://github.com/musistudio/claude-code-router
- **Beschreibung:** Intelligenter Proxy zwischen Claude Code und verschiedenen LLM-Providern. Dynamic Model Switching.
- **Relevanz:** Inspiration fuer Model-Switching-UX

### OpenCode CLI
- **URL:** https://yuv.ai/learn/opencode-cli
- **Beschreibung:** Open-Source Terminal AI-Agent. 75+ LLM-Provider, Ollama, GitHub Copilot, ChatGPT Plus.
- **Relevanz:** Zeigt wie breite Provider-Unterstuetzung aussehen kann

---

## 5. Spec-Driven Development & Roadmap-Tools

### OpenSpec
- **URL:** https://github.com/Fission-AI/OpenSpec
- **Website:** https://openspec.dev/
- **Beschreibung:** Lightweight SDD Framework. Specs leben im Repo (openspec/specs/ + openspec/changes/). CLI-basiert, kein Web-GUI. Works with 20+ AI-Tools.
- **Stars:** 4.100+
- **Relevanz:** Konzeptionelle Vorlage fuer Roadmap-Management. Integration oder Kompatibilitaet anstreben.

### Plane
- **URL:** https://plane.so
- **Beschreibung:** Open-Source Projektmanagement. AI-Features, Roadmaps, Wiki. AGPL-3.0.
- **Staerken:** Modernes UI, native AI, starke Roadmap-Features
- **Luecken:** Kein AI-Coding-Agent
- **Relevanz:** UI/UX-Inspiration fuer Projektmanagement-Teil

### OpenProject
- **URL:** https://www.openproject.org/
- **Beschreibung:** Enterprise PM. GitHub/GitLab-Integration, Version Boards, Roadmaps. GPL v3.
- **Relevanz:** Inspiration fuer SCM-Integration in PM-Kontext

### Ploi Roadmap
- **URL:** https://github.com/ploi/roadmap
- **Beschreibung:** Einfaches Open-Source Roadmap-Tool. /ai Endpoint fuer Machine-readable Data.
- **Relevanz:** Interessanter AI-Endpoint-Ansatz

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
| Roadmap + Agent + Multi-Projekt      | **Keine Loesung**   | **Hauptdifferenzierung**               |
| SVN-Support bei AI-Agents            | **Null**            | **Alleinstellungsmerkmal**             |
| Integrierte Plattform (alle 4 Saeulen) | **Existiert nicht** | **Kernangebot von CodeForge**         |

---

## 8. Strategische Empfehlungen

### Baue auf bestehenden Bausteinen:
- **LLM-Routing:** LiteLLM als Proxy-Layer (statt eigenes Routing)
- **Agent-Backends:** Integration von Aider, OpenHands, SWE-agent als austauschbare Backends
- **Spec-Format:** OpenSpec-Kompatibilitaet fuer Repo-basierte Specs

### Differenziere durch Integration:
- Zentrales Dashboard fuer mehrere Projekte (Git, GitHub, GitLab, SVN)
- Visuelles Roadmap-Management mit bidirektionalem Sync zu Repo-Specs
- LLM-Provider-Management mit Task-basiertem Routing
- Agent-Orchestrierung die verschiedene Coding-Agents koordiniert

### Vermeide:
- Eigenen LLM-Proxy von Grund auf bauen (LiteLLM existiert)
- Eigenen Coding-Agent von Grund auf bauen (integriere bestehende)
- Feature-Krieg mit OpenHands auf deren Kerngebiet (einzelne Issues loesen)

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
