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

### OpenHands (ehemals OpenDevin) — Tiefenanalyse

- **URL:** https://github.com/OpenHands/OpenHands
- **Website:** https://openhands.dev/
- **Paper (ICLR 2025):** https://arxiv.org/abs/2407.16741
- **V1 SDK Paper:** https://arxiv.org/html/2511.03690v1
- **Stars:** 65.000+ | **Lizenz:** MIT (Core), Source-available (Enterprise) | **Sprache:** Python (Backend), TypeScript/React (Frontend)
- **Beschreibung:** Open-Source AI-Driven Development Platform. Naechster direkter Konkurrent zu CodeForge. Web-GUI, CLI, REST+WebSocket-API. Docker/Kubernetes-Deployment. GitHub/GitLab/Bitbucket/Forgejo/Azure DevOps-Integration. Model-agnostisch via LiteLLM (100+ Provider). ICLR 2025 Paper. V0→V1 Architektur-Migration laufend.

#### 1. Architektur & Tech Stack

```
React Frontend (Remix SPA + Vite + TypeScript + Tailwind CSS + Redux + TanStack Query)
        |
        v  REST / WebSocket (FastAPI)
Python Backend (FastAPI, EventStream, AgentController, Session Management)
        |
        v  Docker API / SSH / HTTP
Docker Sandbox (Agent Runtime: Bash, IPython, Browser, File Editor)
```

| Schicht | Technologie | Zweck |
|---|---|---|
| **Frontend** | React + Remix SPA + Vite + TypeScript | Web-GUI mit Redux + TanStack Query State |
| **Backend** | Python + FastAPI | HTTP/WS Server, Session Management, Agent Lifecycle |
| **Agent Runtime** | Python (openhands.sdk) | Agent Loop, LLM Calls, Action/Observation Processing |
| **Sandbox** | Docker Container (pro Session) | Isolierte Code-Ausfuehrung (Bash, IPython, Browser) |
| **LLM** | LiteLLM | Multi-Provider-Abstraktion (100+ Models) |
| **Storage** | FileStore (Local/S3/GCS/InMemory) | Conversation Persistence, State, Events |

**V0 vs. V1 Architektur-Evolution:**
- **V0 (Legacy):** Monolithisch, Sandbox-zentrisch, tight coupling zwischen Agent und Sandbox, SSH-basierte Kommunikation, 140+ Config-Felder in 15 Klassen (2.800 Zeilen)
- **V1 (Aktuell):** Modularer SDK mit klaren Package-Grenzen, opt-in Sandboxing, Workspace-Abstraktion, Event-Sourcing, immutable Config via Pydantic, REST+WebSocket Server built-in

**V1 Package-Struktur:**
| Package | Zweck |
|---|---|
| `openhands.sdk` | Core-Abstraktionen: Agent, Conversation, LLM, Tool, Event System |
| `openhands.tools` | Konkrete Tool-Implementierungen |
| `openhands.workspace` | Execution Environments: Local, Docker, API-Remote |
| `openhands.agent_server` | REST/WebSocket API Server fuer Remote Execution |

#### 2. Kernkonzepte

| Konzept | Beschreibung |
|---|---|
| **Agent** | Untersucht aktuellen State, produziert Actions zum Fortschritt. Verschiedene Implementierungen in AgentHub. |
| **AgentController** | Initialisiert Agent, verwaltet State, treibt den Agent-Loop inkrementell voran. |
| **State** | Datenstruktur mit Task-Info, Step-Count, Event-History, Planning-Data, LLM-Kosten, Delegation-Metadata. |
| **EventStream** | Zentraler Kommunikations-Hub. Publish/Subscribe fuer Actions und Observations. Backbone aller Interaktionen. |
| **Action** | Agent-Anfrage: Shell-Befehl, Python-Code, Browser-Navigation, File-Edit, Agent-Delegation, Nachricht. |
| **Observation** | Umgebungs-Feedback: Command-Output, File-Inhalt, Browser-State, Error Messages. |
| **Runtime** | Fuehrt Actions aus, erzeugt Observations. Sandbox handhabt Befehle in Docker-Containern. |
| **Session** | Haelt genau einen EventStream, einen AgentController, eine Runtime. Repraesentiert einen Task. |
| **ConversationManager** | Verwaltet aktive Sessions, routet Requests zur richtigen Session. |
| **Workspace** | V1-Abstraktion: `LocalWorkspace` (in-process), `RemoteWorkspace` (HTTP), `DockerWorkspace` (Container). |
| **Conversation** | Factory-Pattern: `LocalConversation` oder `RemoteConversation` je nach Workspace-Typ. |

**Agent-Loop (Pseudocode):**
```python
while True:
    prompt = agent.generate_prompt(state)
    response = llm.completion(prompt)
    action = agent.parse_response(response)
    observation = runtime.run(action)
    state = state.update(action, observation)
```

**Datenfluss:**
```
Agent → Actions → AgentController → EventStream → Runtime
Runtime → Observations → EventStream → AgentController → State → Agent
```

#### 3. Agent-System

**Agent-Typen (AgentHub):**

| Agent | Typ | Beschreibung |
|---|---|---|
| **CodeActAgent** | Generalist (Default) | Code schreiben, debuggen, Bash/Python/Browser/File-Edit. Multi-Turn CodeAct Framework. |
| **BrowsingAgent** | Spezialist | Web-Navigation, Formulare, Buttons, komplexe Browser-Interaktion. |
| **Delegator Agent** | Koordinator | Leitet Tasks an Sub-Agents weiter (RepoStudyAgent, VerifierAgent, etc.). |
| **GPTSwarm Agent** | Graph-basiert | Optimierbare Graphen fuer Agent-Systeme, modulare Nodes und Edges. |
| **Micro Agents / Skills** | Spezialisiert | Leichtgewichtige Agents fuer spezifische Tasks, konfiguriert via Markdown + YAML. |

**Multi-Agent-Delegation:**
- `AgentDelegateAction` ermoeglicht hierarchische Agent-Strukturen
- CodeActAgent kann Web-Tasks an BrowsingAgent delegieren
- Sub-Agents operieren als unabhaengige Conversations mit vererbter Config und Workspace

**Skills / Microagents:**
- Spezialisierte Prompts mit Domain-Wissen, gespeichert als Markdown + YAML-Frontmatter
- Drei Trigger-Typen: `always` (immer aktiv), `keyword` (bei Schluesselwort), `manual` (User-gesteuert)
- Speicherort: `.openhands/skills/`, Repo-Root (AGENTS.md, .cursorrules), oder globale Registry
- MCP-Integration: Skills koennen MCP-Server referenzieren fuer zusaetzliche Tools
- Interoperabilitaet: Liest `.cursorrules`, `CLAUDE.md`, `copilot-instructions.md`, `AGENTS.md`

#### 4. Runtime & Sandbox

**Docker Sandbox (Default):**
- Jede Session laeuft in einem eigenen Docker Container
- Volle OS-Capabilities, isoliert vom Host
- SSH-mediated Interface (V0) / HTTP-basiert (V1)
- Container wird nach Session zerstoert (Filesystem-Integritaet)
- Workspace-Mounting fuer projektspezifische Dateien
- Resource Access Policies: Nur Task-relevante Files exponiert

**V1 Workspace-Abstraktion:**
| Workspace-Typ | Ausfuehrung | Use Case |
|---|---|---|
| **LocalWorkspace** | In-process, direkter Host-Zugriff | Schnelles Prototyping, Development |
| **DockerWorkspace** | Container mit Resource-Isolation | Production, Multi-Tenancy |
| **APIRemoteWorkspace** | HTTP-Delegation an Agent Server | Cloud Deployment, SaaS |

**Factory-Pattern:** `Workspace(working_dir="/path")` → LocalWorkspace, `Workspace(host="...", runtime="...")` → RemoteWorkspace

**E2B Integration:**
- Legacy V0: Cloud-Sandbox via E2B (open-source secure environments)
- V1: E2B-Support ueber Workspace-Abstraktion

**Production Observability:**
- VNC Desktop: Echtzeit-GUI-Zugriff auf Agent-Filesystem und Prozesse
- VSCode Web: Eingebetteter Editor im Workspace
- Chromium Browser: Non-headless Browser-Zugriff (Agent sieht was User sieht)

#### 5. LLM-Integration

**Architektur:**
- LiteLLM als Backbone fuer 100+ Provider (OpenAI, Anthropic, Gemini, Bedrock, Ollama, vLLM, etc.)
- Einheitliche `LLM` Klasse im SDK, kapselt Chat- und Completion-APIs
- Support fuer Reasoning-Models (ThinkingBlock fuer Anthropic, ReasoningItemModel fuer OpenAI)

**Multi-LLM-Routing (RouterLLM):**
- `RouterLLM` Subklasse mit custom `select_llm()` Methode
- Per-Invocation LLM-Auswahl (z.B. Text → guenstiges Model, Bilder → Multimodal-Model)
- Fallback/Ensemble-Logik konfigurierbar

**Non-Native Tool Calling:**
- `NonNativeToolCallingMixin`: Text-basierte Prompts + Regex-Parsing fuer Models ohne Function Calling
- Erweitert nutzbares Model-Universum ueber Peer-SDKs hinaus

**Konfiguration:**
- Settings via UI oder `config.toml` (unter `[llm]`)
- Retry-Verhalten konfigurierbar: `num_retries`, `retry_min_wait`, `retry_max_wait`, `retry_multiplier`
- Automatische Rate-Limit-Retries (HTTP 429)
- Custom Tokenizers fuer spezialisierte Models

**Verifizierte Models (SWE-bench Verified, Stand 2025):**
| Model | Resolution Rate |
|---|---|
| Claude Sonnet 4.5 | 72.8% |
| GPT-5 (reasoning=high) | 68.8% |
| Claude Sonnet 4 | 68.0% |
| Qwen3 Coder 480B | 65.2% |

#### 6. Plugin/Extension-System

**Plugin 1.0 Architektur:**
```
plugin/
  ├── skills/          # Skill-Verzeichnisse mit SKILL.md
  ├── runtime.json     # Custom Container Extensions (geplant)
  └── .mcp.json        # MCP Server-Definitionen
```
- Metadata fuer Marketplace GUI (Name, Version, Beschreibung, Autor)
- Plugin-Loading via API: Frontend ruft `app-conversations` API mit Plugin-Spec auf
- Plugins werden im Agent Server (Sandbox) geladen und ausgefuehrt

**MCP (Model Context Protocol) Integration:**
- SSE, SHTTP, und stdio Protokolle unterstuetzt
- Externe Tool-Server registrieren Tools beim Agent
- Konfiguration via UI (Settings > MCP) oder `config.toml` unter `[mcp]`
- Skills koennen MCP-Server referenzieren fuer erweiterte Capabilities

**Tool-System (Action-Execution-Observation):**
| Komponente | Beschreibung |
|---|---|
| **Action** | Input-Schema validiert gegen Pydantic-Model vor Ausfuehrung |
| **Execution** | `ToolExecutor` fuehrt validierte Action aus |
| **Observation** | Strukturiertes Ergebnis mit LLM-kompatiblem Format |
| **Tool Registry** | Entkoppelt Specs von Implementierung, Lazy Instantiation, Distributed Execution |

**Risk Management:**
- Jede Tool-Action bekommt Risk-Level: LOW / MEDIUM / HIGH / UNKNOWN
- `LLMSecurityAnalyzer`: LLM bewertet Sicherheitsrisiko jeder Action
- `ConfirmationPolicy`: Actions ueber User-Threshold erfordern Bestaetigung
- `SecretRegistry`: Credentials in Logs und LLM-Context maskiert
- Auto-Detection von Secrets in Bash-Befehlen und Outputs

#### 7. API (REST + WebSocket)

**REST Endpoints:**

| Endpoint | Methode | Beschreibung |
|---|---|---|
| `/workspaces` | POST | Workspace erstellen |
| `/workspaces/{id}` | GET/DELETE | Workspace Info / Loeschen |
| `/workspaces/{id}/execute` | POST | Befehl ausfuehren |
| `/conversations` | POST | Conversation erstellen |
| `/conversations/{id}` | GET | Conversation abrufen |
| `/conversations/{id}/messages` | POST | Nachricht senden |
| `/conversations/{id}/stream` | GET (WS) | Response-Streaming |
| `/api/user/repositories` | GET | User-Repos auflisten |
| `/api/user/search/repositories` | GET | Repos suchen |
| `/api/user/repository/branches` | GET | Branches auflisten |
| `/api/user/repository/{name}/microagents` | GET | Microagent-Metadata |
| `/api/user/suggested-tasks` | GET | PRs und assigned Issues |

**WebSocket:**
- Bidirektionale Echtzeit-Kommunikation waehrend aktiver Session
- Nachrichten: Agent Actions, Observations, Errors, State Updates
- Streaming von Agent-Events an Frontend

**Authentifizierung:**
- JWT-basiert fuer Session-Identifikation
- FastAPI Dependency Injection fuer Route-Protection
- OAuth 2.0 fuer Git-Provider-Tokens

#### 8. State Management & Persistence

**Event-Sourcing Pattern:**
- Immutable Event-Hierarchie: `Event` → `LLMConvertibleEvent` → `ActionEvent` / `ObservationBaseEvent`
- `ConversationState` als Single Source of Truth (mutable Metadata + append-only EventLog)
- Thread-safe Updates via FIFO Locking
- Deterministic Replay moeglich

**Persistence (Dual-Path):**
- Metadata → `base_state.json` (serialisiert)
- Events → individuelle JSON-Dateien (inkrementell, effizient)

**FileStore Backends:**
| Backend | Beschreibung |
|---|---|
| `InMemoryFileStore` | Ephemerer Storage (Tests, Prototyping) |
| `LocalFileStore` | Lokales Filesystem |
| `S3FileStore` | Amazon S3 Cloud Storage |
| `GoogleCloudFileStore` | Google Cloud Storage |

**Conversation State umfasst:**
- Message History (vollstaendiges Event Log)
- Agent Configuration (LLM Settings, Tools, MCP Servers)
- Execution State (Agent Status, Iteration Count)
- Tool Outputs, Statistics (LLM Usage Metrics)
- Workspace Context, Activated Skills

#### 9. GitHub/GitLab Integration

**Unterstuetzte Provider:**
| Provider | Features | Auth URL Format |
|---|---|---|
| **GitHub** | Repos, Issues, PRs, Installations, Microagent-Discovery | `https://{token}@{domain}/{repo}.git` |
| **GitLab** | Repos, Issues, MRs, Self-Hosted | `https://oauth2:{token}@{domain}/{repo}.git` |
| **Bitbucket** | Repos, Issues, Workspaces | `https://{username}:{password}@{domain}/{repo}.git` |
| **Forgejo** | Repos, Issues (GitHub-kompatible API) | `https://{token}@{domain}/{repo}.git` |
| **Azure DevOps** | Repos, Issues, Organizations | URL-encoded org/project als Username |

**ProviderHandler Architektur:**
- Zentrale Orchestrierung aller Git-Provider-Interaktionen
- `GitService` Protocol: Alle Provider implementieren gemeinsames Interface
- Mixin-basierte Architektur fuer Provider-spezifische Operationen
- Fallback-Pattern: Operationen werden sequenziell ueber alle Provider versucht
- Custom Service Classes via Environment-Variablen (`OPENHANDS_GITHUB_SERVICE_CLS`, etc.)

**GitHub-spezifisch:**
- GitHub App Installation (OpenHands als GitHub App)
- Issue-Labeling mit `@openhands` Tag → Agent arbeitet autonom am Issue
- Automatische PR-Erstellung bei geloestem Issue
- Kommentar mit Zusammenfassung und PR-Link

**Microagent Discovery:**
- Prueft `.cursorrules` im Repo-Root
- Prueft `.openhands/microagents/` Verzeichnis
- Laedt und parsed Microagent-Files ueber `BaseMicroagent.load()`

**MCP-Integration fuer SCM:**
- PR/MR-Erstellung via MCP Tools
- Automatische Links zurueck zu OpenHands Conversations in PR-Descriptions

#### 10. Staerken (was CodeForge lernen sollte)

| Staerke | Detail | Relevanz fuer CodeForge |
|---|---|---|
| **Event-Sourcing + Deterministic Replay** | Immutable Events, Crash Recovery, Time-Travel Debugging | Fuer Agent-Trajectory Recording und Audit-Trail uebernehmen |
| **Workspace-Abstraktion** | Gleicher Agent-Code lokal und remote ausfuehrbar | Fuer Agent Execution Modes (Sandbox/Mount/Hybrid) als Vorbild |
| **Multi-Provider Git-Integration** | 5 Provider mit gemeinsamen Interface + Custom Classes | Provider Registry Pattern bestaetigt, Forgejo/Azure DevOps hinzufuegen |
| **MCP als Erweiterungs-Standard** | Native MCP-Integration fuer Tools und Skills | MCP-Server als Teil des Tool-Systems uebernehmen |
| **LLM Security Analyzer** | LLM bewertet Risiko jeder Agent-Action | Fuer Command Safety Evaluator als zusaetzliche Analyse-Ebene |
| **Skills/Microagent-System** | YAML+Markdown, Trigger-basiert, Repo-spezifisch | Fuer Agent-Spezialisierung und Task-spezifische Prompts |
| **Non-Native Tool Calling** | Models ohne Function Calling trotzdem nutzbar | Fuer LLM Capability Levels (Reine Completion Models) direkt relevant |
| **RouterLLM** | Per-Invocation Model-Auswahl | Bestaetigt Scenario-based Routing Ansatz |
| **Plugin Marketplace (geplant)** | Skills + MCP + Runtime Extensions als Package | Fuer CodeForge Community-Extensions als Inspiration |
| **Stuck Detection** | Erkennung von Endlos-Schleifen und redundanten Calls | Fuer Agent-Workflow Quality Layer uebernehmen |
| **Benchmark-Evaluation** | 15+ Benchmarks, SWE-bench State-of-the-Art | Evaluation-Framework fuer eigene Agent-Qualitaetsmessung |
| **3-Tier Testing** | Programmatisch (Commit) + LLM-Integration (Daily) + Benchmark (On-Demand) | Testing-Strategie fuer Python Workers uebernehmen |
| **Context Condensation** | Automatische Context-Window-Optimierung | Fuer History Processors / Context-Window-Management |
| **Pause/Resume** | State Persistence mit Event-Sourcing | Fuer langlebige Agent-Tasks und Plan→Approve Workflow |

#### 11. Schwaechen / Luecken die CodeForge fuellt

| Schwaeche | Detail | CodeForge-Loesung |
|---|---|---|
| **Kein Roadmap/Feature-Map-Management** | Keine visuelle Roadmap, kein Spec-Tracking, kein Feature-Planning | Roadmap/Feature-Map als Kernsaeule mit Auto-Detection + Multi-Format-Support |
| **Kein Multi-Projekt-Dashboard** | Ein Projekt pro Session, keine uebergreifende Verwaltung | Projekt-Dashboard fuer mehrere Repos gleichzeitig |
| **Kein SVN-Support** | Nur Git-basierte SCMs (GitHub, GitLab, Bitbucket, Forgejo, Azure DevOps) | SVN als First-Class SCM-Provider |
| **Single-Task-Fokus** | Eine Session = ein Task, keine Orchestrierung mehrerer paralleler Tasks | Multi-Agent-Orchestrierung ueber mehrere Tasks/Repos |
| **Nur Python-Backend** | Gesamter Core in Python (Performance-Limitierung bei hoher Concurrency) | Go Core fuer HTTP/WS/Scheduling, Python nur fuer AI-Arbeit |
| **Kein Scenario-basiertes Routing** | RouterLLM existiert, aber keine Task-Typ-basierte Model-Auswahl | Scenario-Routing via LiteLLM Tags (default/background/think/longContext/review/plan) |
| **Keine PM-Tool-Integration** | Kein Sync mit Plane, OpenProject, Jira, Linear (nur geplant) | Bidirektionaler Sync mit PM-Tools als Kernsaeule |
| **Keine Spec-Driven Development** | Keine Auto-Detection von OpenSpec, Spec Kit, Autospec | Drei-Tier Auto-Detection + Multi-Format SDD-Support |
| **Hohe LLM-Kosten** | Frontier Models erforderlich, Looping-Verhalten treibt Kosten | Budget-Enforcement pro Task/Projekt/User, Experience Pool fuer Caching |
| **Ambiguitaet-Problem** | Schlecht bei vagen Anforderungen ohne klare Specs | Dokument-Pipeline PRD→Design→Tasks→Code reduziert Ambiguitaet |
| **Kein ADR/RFC-Support** | Keine Erkennung/Integration von Architektur-Entscheidungen | Auto-Detection von docs/adr/, docs/rfcs/ |
| **Kein Feature-Flag-Integration** | Kein Wissen ueber Feature-Rollout-Status | Integration mit Unleash, OpenFeature, Flagsmith |
| **Eingeschraenkte Agent-Vielfalt** | Nur eigene Agents (CodeAct, Browsing, Micro), keine externen Agents | Integration von Aider, OpenHands, SWE-agent als austauschbare Backends |
| **Kein Gitea/Forgejo als SCM-Adapter** | Forgejo nur als Git-Provider, nicht als PM-Tool-Adapter | Gitea/Forgejo Issues/Boards als PM-Sync-Ziel |
| **Keine Desktop-IDE-Integration** | Nur Web-GUI, kein VS Code Extension, kein JetBrains Plugin | Anbindbar ueber MCP Server (geplant) |

#### 12. Uebernommene Patterns fuer CodeForge

| Pattern | Quelle in OpenHands | Umsetzung in CodeForge |
|---|---|---|
| Event-Sourcing State Model | ConversationState + EventLog | Trajectory Recording mit immutable Events und Replay |
| Workspace Factory Pattern | `Workspace()` → Local/Docker/Remote | Agent Execution Modes: Sandbox/Mount/Hybrid mit Factory |
| Action-Execution-Observation | Tool System mit Pydantic-Validierung | Tool-Bundles mit Schema-Validierung in YAML |
| GitService Protocol + ProviderHandler | Mixin-basierte Multi-Provider Git-Integration | Provider Registry Pattern (gleiche Architektur, erweitert um SVN) |
| Skills mit YAML-Frontmatter + Triggers | Microagents mit always/keyword/manual Trigger | Agent-Spezialisierung als YAML mit Trigger-Konfiguration |
| LLM Security Analyzer | Risk-Level pro Action + ConfirmationPolicy | Command Safety Evaluator mit Risk-Levels + Approval-Flow |
| SecretRegistry + Auto-Masking | Credentials in Logs und LLM-Context maskiert | Secret-Masking in Agent-Execution und Trajectory Logs |
| Stuck Detection | Erkennung redundanter Calls und Loops | Quality Layer mit Loop-Detection und Re-Planning (MagenticOne) |
| Context Condensation | Automatische Context-Window-Optimierung | History Processors Pipeline (Buffered/TokenLimited/HeadAndTail) |
| Multi-Format Prompt Interop | Liest .cursorrules, CLAUDE.md, AGENTS.md | Context File Interoperability fuer alle unterstuetzten Formate |
| Non-Native Tool Calling | Text-Prompts + Regex fuer Models ohne Function Calling | LLM Capability Levels: Reine Completion → alles (Context, Tools, Quality) |

#### 13. Explizit NICHT uebernommen

| Konzept | Grund |
|---|---|
| Python-only Backend | Go Core fuer Performance, Python nur fuer AI-spezifische Arbeit |
| React Frontend | SolidJS gewaehlt (leichter, performanter, kein VDOM) |
| Redux + TanStack Query | SolidJS hat eigene Reactive Primitives |
| FastAPI als HTTP Server | Go net/http fuer Core (Performance, Concurrency) |
| SSH-basierte Sandbox-Kommunikation | Message Queue (NATS/Redis) zwischen Go Core und Python Workers |
| Single-Session-Architektur | Multi-Projekt-Dashboard mit parallelen Sessions |
| Event-Sourcing als einziger Persistenz-Mechanismus | Zusaetzlich PostgreSQL fuer strukturierte Daten (Projekte, User, Config) |
| Plugin Marketplace Ansatz | Fokus auf Provider Registry Pattern + YAML-basierte Erweiterbarkeit |

### Open SWE (LangChain)
- **URL:** https://github.com/langchain-ai/open-swe
- **Beschreibung:** Cloud-basierter async Coding-Agent. Versteht Codebases, plant Loesungen, erstellt PRs automatisch.
- **Staerken:** GitHub-Integration, async Workflows
- **Luecken:** Kein Multi-Provider-LLM-Management, kein Roadmap-Feature, kein Self-Hosting-Fokus

### Codel
- **URL:** https://github.com/semanser/codel
- **Stars:** ~2.400 | **Lizenz:** AGPL-3.0 | **Sprache:** Go (Backend), React (Frontend)
- **Beschreibung:** Vollautonomer AI-Agent mit Terminal, Browser und Editor in sandboxed Docker-Umgebung. Modernes Self-Hosted Web-UI, persistente History in PostgreSQL, automatische Docker-Image-Auswahl basierend auf User-Tasks.
- **Kernfeatures:**
  - Built-in Browser fuer Web-Research waehrend Tasks
  - Built-in Text-Editor mit File-Change-Visualisierung
  - Smart Docker Image Picker (Task-basierte Auswahl)
  - Self-Contained Sandboxed Execution
- **Relevanz fuer CodeForge:** **Hoch als Architektur-Referenz.** Architektonisch sehr nah an CodeForge: Go-Backend, Web-GUI, Docker-Sandbox. Fehlt: Multi-Projekt, Roadmap, Multi-Agent-Orchestrierung.

### AutoForge
- **URL:** https://github.com/AutoForgeAI/autoforge
- **Stars:** ~1.600 | **Sprache:** Python (Agent), React (UI)
- **Beschreibung:** Langzeit-autonomer Coding-Agent basierend auf Claude Agent SDK. Baut komplette Applikationen ueber mehrere Sessions via Two-Agent-Pattern (Initializer + Coding Agent). React-basiertes UI fuer Real-Time-Monitoring.
- **Kernfeatures:**
  - Two-Agent-Architektur: Initializer generiert Feature-Test-Cases, Coding Agent implementiert
  - Multi-Session Design (Tasks ueber Stunden/mehrere Sessions)
  - Claude, GLM (Zhipu AI), Ollama, Kimi (Moonshot), Custom Providers
- **Relevanz fuer CodeForge:** **Mittel.** Multi-Session, Test-First-Ansatz als Pattern-Referenz fuer CodeForge's Plan→Approve→Execute→Review→Deliver Workflow.

### bolt.diy (Community Fork von Bolt.new)
- **URL:** https://github.com/stackblitz-labs/bolt.diy
- **Stars:** ~19.000 | **Lizenz:** MIT (WebContainers API erfordert kommerzielle Lizenz fuer Production) | **Sprache:** TypeScript (Remix)
- **Beschreibung:** Offizieller Open-Source-Fork von Bolt.new. Prompt, Run, Edit und Deploy von Full-Stack-Web-Apps mit beliebigem LLM. 19+ AI-Provider-Integrationen (OpenAI, Anthropic, Ollama, OpenRouter, Gemini, LM Studio, Mistral, DeepSeek, etc.).
- **Kernfeatures:**
  - In-Browser Full Dev Environment (Filesystem, Node Server, Terminal, Package Manager via WebContainers)
  - 19+ LLM Provider, MCP Integration, Git Integration, Diff View
  - Expo App Creation fuer React Native
  - Electron Desktop App Option
- **Relevanz fuer CodeForge:** **Mittel.** Zielt auf "Vibe Coding" und App-Erstellung, nicht auf Multi-Projekt-Management oder Agent-Orchestrierung. Multi-LLM-Provider-Architektur als Referenz.

### Dyad
- **URL:** https://github.com/dyad-sh/dyad
- **Stars:** ~16.800 | **Lizenz:** Apache 2.0 (mit `src/pro` Directory ausgenommen) | **Sprache:** TypeScript (Electron)
- **Beschreibung:** Lokaler, Open-Source AI App Builder (v0/Lovable/Replit/Bolt-Alternative). Alles laeuft lokal auf der eigenen Maschine. Real-Time-Previews, Instant Undo.
- **Kernfeatures:**
  - Vollstaendig lokale Ausfuehrung (nichts verlaesst die Maschine)
  - Multi-Model-Support (OpenAI, Google, Anthropic, freie Modelle)
  - Real-Time Previews, Instant Undo, Responsive Workflows
- **Relevanz fuer CodeForge:** **Niedrig-Mittel.** Fokus auf App-Erstellung von Grund auf, nicht Multi-Projekt oder Agent-Orchestrierung. Local-First-Philosophie passt zu CodeForge's Self-Hosted-Ansatz.

### CLI Agent Orchestrator (CAO) — AWS
- **URL:** https://github.com/awslabs/cli-agent-orchestrator
- **Stars:** ~210 | **Lizenz:** Apache 2.0 | **Sprache:** Python
- **Beschreibung:** AWS-gestuetztes Open-Source Multi-Agent-Orchestrierungs-Framework. Transformiert Developer-CLI-Tools (Amazon Q CLI, Claude Code, etc.) in ein hierarchisches Multi-Agent-System. Supervisor-Agent koordiniert spezialisierte Worker-Agents in isolierten tmux-Sessions.
- **Kernfeatures:**
  - Hierarchisches Supervisor/Worker Agent Pattern
  - Isolierte tmux-Sessions pro Agent mit MCP-Kommunikation
  - Drei Orchestrierungs-Patterns: Handoff (synchron), Assign (async parallel), Send Message (direkt)
  - Flow Scheduling (Cron-aehnlich) fuer unbeaufsichtigte automatische Ausfuehrung
  - Claude Code, Amazon Q CLI; geplant: Codex CLI, Gemini CLI, Aider
  - Vollstaendig lokale Ausfuehrung
- **Relevanz fuer CodeForge:** **Sehr hoch — Konkurrent UND Architektur-Referenz.** Hierarchische Multi-Agent-Orchestrierung via tmux/MCP ist direkt vergleichbar mit CodeForge's Agent-Orchestrierungs-Layer. Supervisor/Worker-Pattern, isolierte Sessions, Support fuer mehrere CLI-Agents. Cron-basiertes Flow Scheduling relevant. Fehlt: Web-GUI, Projekt-Dashboard, Roadmap-Features.

---

## 2. AI Coding Agents (Partial Overlap)

### SWE-agent — Tiefenanalyse

- **URL:** https://github.com/SWE-agent/SWE-agent
- **Paper (NeurIPS 2024):** https://arxiv.org/abs/2405.15793
- **Mini-SWE-Agent:** https://github.com/SWE-agent/mini-swe-agent
- **Stars:** ~15.000+ | **Lizenz:** MIT | **Sprache:** Python
- **Autoren:** John Yang, Carlos E. Jimenez, Alexander Wettig, Kilian Lieret, Shunyu Yao, Karthik Narasimhan, Ofir Press (Princeton / Stanford)
- **Status:** Aktiv, v1.0 Release, NeurIPS 2024 Paper

#### 1. Architektur

```
GitHub Issue / User Task
        |
        v
  SWE-agent Runner (sweagent/run/)
        |
        ├── Agent (ReAct Loop)
        │     ├── Thought → Action → Observation
        │     ├── LLM Call (via LiteLLM)
        │     └── History Processors (Context-Window-Optimierung)
        |
        ├── Tool System (sweagent/tools/)
        │     ├── ToolConfig (Bundle-Laden, Registrierung)
        │     ├── ToolHandler (Execution, Filtering, Security)
        │     └── Tool Bundles (YAML-basierte Tool-Definitionen)
        |
        └── Environment (SWE-ReX)
              ├── Docker Container (isoliert)
              ├── Bash Execution (Actions → Shell)
              └── State Management (/root/state.json)
```

- **Kern-Pattern:** Agent-Computer Interface (ACI) — speziell fuer LLMs entworfene Shell-Befehle
- **Runtime:** SWE-ReX (Remote Execution) — Docker-basierte Sandbox
- **LLM-Integration:** LiteLLM fuer 100+ Provider-Support

#### 2. Kernkonzepte

- **Agent-Computer Interface (ACI):**
  - Zentrale Innovation des Papers: Traditionelle Unix-Shells sind fuer LLMs ungeeignet
  - Spezielle Befehle (`find_file`, `search_file`, `search_dir`, `edit`, `scroll_up/down`)
  - Befehle optimiert fuer LLM-Verstaendnis: kompakte Ausgabe, klare Fehlermeldungen
  - Inspiriert von HCI-Forschung (Human-Computer Interaction → Agent-Computer Interaction)

- **ReAct Loop:**
  - Bei jedem Schritt: LLM generiert Thought + Action
  - Action wird in der Umgebung ausgefuehrt → Observation
  - Observation fliesst zurueck in den naechsten LLM-Call
  - Typisches Muster: Lokalisierung (Turns 1-5) → Edit+Execute-Loops (Turns 5+)
  - Submission via `submit`-Befehl erzeugt Patch (git diff)

- **Tool-System:**
  - **ToolConfig:** Laedt Tools aus Bundles, erkennt Duplikate, konvertiert in Function-Calling-Format
  - **ToolHandler:** Sicherheit via `ToolFilterConfig` — Blocklist (vim, nano, gdb), Standalone-Blocklist (python, bash, su), Conditional Blocking (regex-basiert)
  - **Tool Bundles:** YAML-definierte Tool-Sammlungen, installiert in `/root/tools/{bundle_name}`, mit optionalem `install.sh` und PATH-Update
  - **15+ vordefinierte Bundles** fuer verschiedene Aufgaben
  - **Multiline-Support:** Heredoc-Style (`<< '{end_name}'`) fuer mehrzeilige Befehle

- **History Processors:**
  - Pipeline zur Context-Window-Optimierung
  - Aeltere Observations werden gekuerzt/zusammengefasst
  - Nur aktuelle Arbeitskontext bleibt vollstaendig
  - Vermeidet Token-Limit-Ueberschreitung bei langen Sessions

- **State Management:**
  - Environment-State in `/root/state.json`
  - State-Commands nach jeder Action (Working Directory, Variablen)
  - Ermoeglicht Introspection des Environment-Zustands

#### 3. SWE-ReX (Remote Execution)

- Separates Modul fuer sandboxed Code-Execution
- Docker-Container pro Task (isoliert)
- Asynchrone Bundle-Installation
- PATH-Erweiterung pro Bundle
- `which`-Checks zur Verifikation der Tool-Verfuegbarkeit
- Unterstuetzt lokale und Remote-Execution

#### 4. Mini-SWE-Agent

- **100 Zeilen Python** — radikal minimalistischer Agent
- **>74% auf SWE-bench Verified** (aktuell, mit Gemini 3 Pro)
- **65% auf SWE-bench Verified** (initial, mit Claude Sonnet)
- **Kein Tool-Calling-Interface** — nur Bash als einziges Tool
- **Lineare History** — jeder Schritt wird an die Messages angehaengt
- **`subprocess.run`** — jede Action unabhaengig (kein stateful Shell)
- **Kernerkenntnis:** Moderne LLMs brauchen weniger Scaffolding als angenommen
- **Nutzer:** Meta, NVIDIA, Essential AI, Anyscale
- Von Princeton/Stanford-Team (gleiche Autoren wie SWE-agent)

#### 5. SWE-bench Performance

| Konfiguration | SWE-bench Verified | Jahr |
|---|---|---|
| SWE-agent + GPT-4 (1106) | 12.5% (SWE-bench full) | 04/2024 |
| SWE-agent + Claude 3 Opus | 12.5% (SWE-bench full) | 04/2024 |
| SWE-agent + Claude 3.5 Sonnet | ~33% | 06/2024 |
| SWE-agent 1.0 + Claude 3.7 Sonnet | ~66% | 02/2025 |
| Mini-SWE-Agent + Gemini 3 Pro | >74% | 2025 |
| Claude Opus 4.5 + Live-SWE-agent | 79.2% | 2025 |

- **SWE-bench Pro** (haerterer Benchmark): Beste Modelle nur ~23% (GPT-5, Claude Opus 4.1)
- **Kontaminations-Bedenken:** Zunehmende Evidenz, dass Frontier-Models SWE-bench-Daten im Training gesehen haben → neue Benchmarks (SWE-bench Pro, SWE-rebench, SWE-bench-Live)

#### 6. Hook-System

- Observer-Pattern fuer Agent-Lifecycle-Events
- Hooks fuer: Pre/Post-Action, Error-Handling, Submission, State-Changes
- Erweiterbar fuer Logging, Observability, Custom-Logic
- Aehnlich zu CodeForge's geplantem Hook-System

#### 7. Staerken

| Staerke | Detail |
|---|---|
| **ACI-Innovation** | Speziell fuer LLMs entworfene Shell-Befehle statt generische Unix-Tools |
| **Akademisch fundiert** | NeurIPS 2024 Paper, Princeton/Stanford Forschung |
| **SWE-bench Benchmark** | De-facto Standard fuer Coding-Agent-Evaluation |
| **Tool-Bundles** | YAML-deklarativ, erweiterbar, austauschbar |
| **Mini-SWE-Agent** | Beweist, dass 100 Zeilen ausreichen fuer 74% SWE-bench |
| **History Processors** | Robuste Context-Window-Verwaltung |
| **Cybersecurity-Modus** | Kann auch fuer Offensive Security und CTF genutzt werden |
| **SWE-ReX Sandbox** | Docker-basierte Isolation, Remote Execution |
| **LiteLLM-Integration** | 100+ Provider out-of-the-box |
| **Open Source (MIT)** | Volle Freiheit fuer Integration und Anpassung |

#### 8. Schwaechen

| Schwaeche | Detail |
|---|---|
| **Edit-Fehlerrate** | 51.7% der Trajectories haben 1+ fehlgeschlagene Edits |
| **Keine Web-GUI** | Reines CLI-Tool, kein Dashboard |
| **Single-Agent** | Kein Multi-Agent-Pattern, kein Delegation |
| **Kein Multi-Projekt** | Ein Issue pro Run, keine Projekt-Verwaltung |
| **Kein Approval-Flow** | Rein autonom, kein Human-in-the-Loop |
| **Kontaminations-Risiko** | SWE-bench-Scores moeglicherweise durch Training-Kontamination beeinflusst |
| **Context-Window-Abhaengig** | Performance korreliert stark mit verfuegbarem Context (8k vs 128k) |
| **Kein State Persistence** | Kein Session-uebergreifendes Gedaechtnis |

#### 9. Relevanz fuer CodeForge

```
Go Core → Task Queue → Python AI Worker → SWE-agent (CLI / Python API)
   ├── ACI Tools: find_file, search_file, edit (Code-Navigation)
   ├── SWE-ReX Sandbox: Docker-Container (Isolation)
   ├── Tool Bundles: YAML-basiert (erweiterbar)
   ├── History Processors: Context-Window-Optimierung
   ├── ReAct Loop: Thought → Action → Observation
   └── LLM Call: via LiteLLM (gleicher Stack wie CodeForge)
```

- **Backend-Kandidat Prio 2** (nach Aider wegen geringerem API-Maturity)
- ACI-Konzept direkt uebernehmbar fuer eigene Tool-Definitionen
- Tool-Bundles-Konzept identisch mit CodeForge's geplantem YAML-Tool-System
- History Processors uebernehmbar fuer Context-Window-Strategien
- SWE-bench als Evaluation-Framework fuer CodeForge's Agent-Qualitaet
- Mini-SWE-Agent als Referenz fuer minimal viable agent scaffolding

#### 10. Uebernommene Patterns

| Pattern | Anwendung in CodeForge |
|---|---|
| ACI (Agent-Computer Interface) | Tool-Definitionen optimiert fuer LLM-Verstaendnis |
| Tool-Bundles (YAML) | Deklarative Tool-Definitionen, austauschbare Bundles |
| History Processors | Context-Window-Pipeline (Buffered, TokenLimited, HeadAndTail) |
| ReAct Loop | Thought→Action→Observation als Agent-Grundmuster |
| ToolFilterConfig | Command Safety Evaluator (Blocklist, Conditional Blocking) |
| State Management (/root/state.json) | Worker-State-Tracking in Agent-Containern |
| SWE-ReX Sandbox | Docker-basierte Agent-Execution-Isolation |
| Mini-SWE-Agent Pattern | Minimal-Scaffolding als Fallback fuer einfache Tasks |

#### 11. Explizit NICHT uebernommen

| Konzept | Grund |
|---|---|
| Reine CLI-Architektur | CodeForge ist Web-GUI-basiert |
| Single-Agent-Pattern | CodeForge: Multi-Agent-Orchestrierung |
| Fehlender Approval-Flow | Human-in-the-Loop ist Kernprinzip |
| Stateless Sessions | CodeForge braucht persistente Projekt-Kontexte |
| SWE-bench als einzige Metrik | CodeForge evaluiert auch Kosten, Speed, User-Zufriedenheit |

### Devika
- **URL:** https://github.com/stitionai/devika
- **Stars:** ~19.500 | **Lizenz:** MIT | **Status:** Experimentell/Stagniert (Rebranding zu "Opcode" angekuendigt, aber wenig Aktivitaet seit Mitte 2025)
- **Beschreibung:** Erste Open-Source-Implementierung eines Agentic Software Engineer (Devin-Alternative). Python-Backend (Flask + SocketIO), Svelte-Frontend, Multi-LLM (Claude 3, GPT-4, Gemini, Mistral, Groq, Ollama), AI-Planning mit spezialisierten Sub-Agents, Web-Browsing via Playwright, Multi-Language Code Generation. SQLite-Datenbank fuer Persistenz. Jinja2-Prompt-Templates.
- **Kernkonzepte:**
  - **Agent Core (Orchestrator):** Zentrale `Agent`-Klasse treibt den Planning/Execution-Loop. Verwaltet Conversation History, Agent State, Context Keywords. Delegiert an spezialisierte Sub-Agents.
  - **Spezialisierte Sub-Agents (9 Stueck):**
    - *Planner:* Analysiert User-Prompt, generiert Step-by-Step-Plan mit Focus-Areas
    - *Researcher:* Extrahiert Search-Queries aus dem Plan, priorisiert fuer Effizienz
    - *Coder:* Transformiert Plan + Research in Code (Multi-File, Multi-Language)
    - *Action:* Mappt Follow-up-Prompts auf Action-Keywords (run, test, deploy, fix, implement, report)
    - *Runner:* Fuehrt generierten Code in Sandbox-Umgebung aus (Multi-OS)
    - *Feature:* Implementiert neue Features in bestehenden Code
    - *Patcher:* Debuggt und fixt Issues mit Root-Cause-Analyse
    - *Reporter:* Generiert Projekt-Dokumentation als PDF
    - *Decision:* Handhabt Spezial-Commands (Git-Ops, Browser-Sessions)
  - **Agent-Loop (Zwei Phasen):**
    - *Initial Execute:* Prompt → Planner → Researcher → Web Search (Bing/Google/DuckDuckGo) → Crawler → Formatter → Coder → Code auf Disk
    - *Subsequent Execute:* Follow-up → Action Agent → Spezialist (Runner/Feature/Patcher/Reporter) → Update
  - **Browser Interaction:** Playwright-basiert. `Browser`-Klasse fuer High-Level-Primitives (navigate, query DOM, extract text/markdown/PDF, screenshots). `Crawler`-Klasse fuer LLM-gesteuerte Webpage-Interaktion (Reasoning-Loop: Page Content + Objective → LLM → Action wie CLICK/TYPE/SCROLL).
  - **Knowledge Base:** SentenceBERT fuer semantische Keyword-Extraction. Domain-spezifische Experts (WebDev, Physics, Chemistry, Mathematics) als Wissens-Module.
  - **State Management:** `AgentStateModel` in SQLite — sequentielle State-Logs mit Step, Internal Monologue, Browser Session (Screenshot + URL), Terminal Session (Command + Output), Token Usage, Timestamp. Ermoeglicht Real-Time-Visualisierung des Agent-Denkprozesses.
  - **Architektur-Stack:**
    - *Backend:* Python 3.10-3.11, Flask, Flask-SocketIO (Port 1337)
    - *Frontend:* SvelteKit + Bun (Port 3001)
    - *Kommunikation:* Socket.IO (WebSocket) fuer Real-Time + REST API (/api/*)
    - *Datenbank:* SQLite (Projects + AgentState Tabellen)
    - *Browser:* Playwright
    - *LLM:* Direkte API-Calls pro Provider (kein einheitlicher Proxy)
  - **Jinja2-Prompt-Templates:** Jeder Sub-Agent hat eigene `prompt.jinja2` — Prompts als separate Dateien, nicht im Code
  - **Stateless Agents:** Agents sind stateless/idempotent — State wird vom Agent Core verwaltet und bei Bedarf uebergeben
  - **Externe Integrationen:** GitHub (Clone, File-List, Commits), Netlify (Deploy mit URL-Generierung)
  - **Config:** `config.toml` fuer API-Keys, Pfade, Such-Engine-Auswahl
- **Staerken:**
  - Konzeptionell saubere Agent-Trennung — jeder Sub-Agent hat klar definierte Aufgabe
  - Jinja2-Prompt-Templates (identisches Pattern wie CodeForge plant)
  - SentenceBERT Keyword-Extraction fuer kontextbewusstes Research
  - Real-Time Agent-State-Visualisierung (Internal Monologue, Step, Browser, Terminal)
  - Multi-LLM inkl. Ollama fuer lokale Models
  - Open-Source (MIT), Community-getrieben
  - Modularer, erweiterbarer Aufbau
  - Agent-Loop-Pattern (Plan→Research→Code→Execute) als Referenzarchitektur
- **Schwaechen:**
  - Projekt de facto stagniert/aufgegeben — Issue #685 "is this project abandoned?" ohne Antwort, kaum Commits seit Mitte 2025
  - Viele Features unimplementiert oder broken (offiziell im README dokumentiert)
  - Kein Human-in-the-Loop — Agent laeuft ohne Approval-Flow
  - Keine Checkpoint/Rollback-Mechanismen
  - Single-Process Flask Server, kein Scaling, kein Message Queue
  - Kein Diff-basiertes File-Editing — Code wird direkt geschrieben
  - Sicherheitsluecken (API-Key-Exposure-Risiko in Config)
  - Kein Context-Window-Management — voller Kontext wird an LLM geschickt
  - Direkte Provider-API-Calls statt einheitlichem LLM-Proxy
  - Kein Git-Integration fuer Code-Changes (nur GitHub-Clone)
  - Kein Projektmanagement oder Roadmap
- **Relevanz fuer CodeForge:**
  - **Sub-Agent-Architektur:** Planner/Researcher/Coder/Patcher-Trennung als Muster fuer CodeForge Worker-Module (nicht 1:1, aber konzeptionell)
  - **Jinja2-Prompt-Templates:** Bestaetigt CodeForge-Entscheidung, Prompts als separate Template-Dateien zu verwalten
  - **SentenceBERT Keywords:** Direkt uebernommen — KeyBERT fuer semantische Keyword-Extraction im Research-Modul
  - **Agent State Visualization:** Real-Time-Anzeige von Internal Monologue, Steps, Browser, Terminal als Muster fuer CodeForge Dashboard
  - **Browser Crawling Pattern:** LLM-gesteuerter Crawler (Page Content → LLM → Action) als Inspiration fuer Web-Research-Worker
  - **Anti-Patterns (was CodeForge vermeidet):** Kein Approval-Flow, kein Checkpointing, Single-Process, kein Context-Management, direkte Provider-Calls

### Aider
- **URL:** https://aider.chat / https://github.com/Aider-AI/aider
- **Stars:** ~40.000+ | **Lizenz:** Apache 2.0 | **Sprache:** Python
- **Beschreibung:** Terminal-basierter AI Pair-Programmer. Git-nativ, Multi-Model-Support (127+ Provider via LiteLLM). tree-sitter + PageRank Repo Map fuer Codebase-Kontext. 7+ Edit-Formate (modellspezifisch optimiert). Architect/Editor Zwei-Modell-Pattern. Auto-Lint, Auto-Test, Feedback-Loop mit Reflection Cycles. CLI-Scripting und inoffizielle Python API.
- **Staerken:** Ausgereiftestes Codebase-Kontext-System (Repo Map), tiefste Git-Integration aller Tools, empirisch optimierte Edit-Formate, Architect/Editor Reasoning-Trennung
- **Luecken:** Kein Web-GUI (nur experimentelle Browser-UI), kein Projektmanagement, keine REST API, keine Agent-Orchestrierung, keine Sandbox-Isolation, Single-User/Single-Session
- **Relevanz:** Potentielles Agent-Backend fuer CodeForge (via `--message` CLI oder Python API). Repo Map Konzept als Inspiration fuer GraphRAG Context Layer.
- **Detaillierte Analyse:** [docs/research/aider-deep-analysis.md](aider-deep-analysis.md)

### Cline
- **URL:** https://cline.bot / https://github.com/cline/cline
- **Stars:** 4M+ Nutzer | **Lizenz:** Apache 2.0 | **Version:** 3.17+ (aktiv entwickelt)
- **Beschreibung:** Autonomer AI-Coding-Agent als VS Code Extension (+ CLI + JetBrains). Erstellt/editiert Files, fuehrt Befehle aus, steuert Browser, nutzt MCP-Tools — mit Human-in-the-Loop Approval bei jedem Schritt. Zero Server-Side Components — alles laeuft lokal. React-Webview-Frontend, TypeScript-Extension-Backend, gRPC-Kommunikation ueber Protocol Buffers.
- **Kernkonzepte:**
  - **Drei-Tier-Runtime-Architektur:**
    - *VS Code Extension Host (Node.js):* Core-Orchestrierung, Task-Management, State-Persistenz
    - *Webview UI (React):* Sandboxed Browser-Interface im VS Code Panel
    - *CLI Tool (Go):* Standalone Terminal-Interface mit geteilten Protokollen
  - **Controller (Central Orchestrator):** Singleton, verwaltet Task-Lifecycle, StateManager, AuthService, McpHub. Registriert 60+ VS Code Commands.
  - **Task Execution Engine (Recursive Loop):**
    - User Input → Controller → Task Instance → ContextManager baut System-Prompt
    - ApiHandler streamt AI-Response → Task parst Tool-Invocations
    - Ask/Say Pattern: User Approval anfordern → ToolExecutor fuehrt aus
    - Ergebnis wird an Conversation History angehaengt → Loop bis Completion
  - **Tool-System (5 Kategorien):**
    - *File Operations:* read_file, write_to_file, replace_in_file, list_files, search_files — mit Diff-basiertem Approval
    - *Terminal Commands:* execute_command mit Output-Monitoring, `requires_approval`-Flag per LLM
    - *Browser Automation:* Browser starten, Screenshots, Page-Interaktion
    - *MCP Tools:* Tools von verbundenen Model Context Protocol Servern
    - *Context Management:* Workspace-Context in Prompts einbauen, .clinerules laden
  - **Plan/Act Mode System:**
    - *Plan Mode:* Read-Only Codebase-Analyse und Architektur-Planung. Kann guenstigeres Model nutzen.
    - *Act Mode:* Echte Code-Aenderungen ausfuehren. Separates Model konfigurierbar.
    - Expliziter User-Toggle — Agent kann nicht selbst in Act Mode wechseln
    - Kostenoptimierung: DeepSeek-R1 fuer Plan, Claude Sonnet fuer Act → bis 97% Kostenreduktion
  - **Human-in-the-Loop Approval (Layered Permission System):**
    - *Default:* Jede Aktion braucht User-Approval (File Read, Write, Command, Browser, MCP)
    - *Auto-Approve Menu:* Granulare Autonomie pro Tool-Kategorie (Read Files, Edit Files, Safe Commands, Browser, MCP)
    - *`.clinerules`:* Projektspezifische Regeln als Textdatei oder Verzeichnis mit Markdown-Files — steuern Approval-Verhalten, Coding-Standards, Architektur-Constraints
    - *Workflows:* On-demand Automation (`/workflow.md`), werden als `<explicit_instructions>` injiziert — verbrauchen Tokens nur bei Aufruf
    - *YOLO Mode:* Alle Approvals umgehen (mit OS-Notifications als Safety-Net)
  - **Checkpoint System (Shadow Git):**
    - Separates Git-Repository (unsichtbar fuer User's normalen Git-Workflow)
    - Automatische Commits nach jeder AI-Operation
    - Zwei Diff-Modi: Incremental (letzter→aktueller Checkpoint) und Full Task (Baseline→Final)
    - Restore Files Only (Code zuruecksetzen, Chat behalten) oder Restore Files & Task (beides zurueck)
    - Nested-Git-Handling: Temporaeres Renaming (`_disabled` Suffix) bei Operationen
    - `.gitignore` und `.gitattributes` (LFS) werden respektiert
  - **MCP (Model Context Protocol) Integration:**
    - McpHub verwaltet Server-Connections via `mcp_config.json`
    - Transport: stdio, SSE, HTTP Streaming
    - Cline kann eigene MCP-Server erstellen ("add a tool" → baut Server + installiert in Extension)
    - MCP Rules: Auto-Selektion basierend auf Konversations-Kontext und Keywords
    - Global Workflows (v3.17+): Workflows ueber alle Workspaces teilen
    - MCP Marketplace fuer Community-Server
  - **API Handler Architektur (40+ Provider):**
    - Factory Pattern: `buildApiHandler()` erstellt provider-spezifische Handler
    - Einheitliches `ApiHandler`-Interface fuer alle Provider
    - Streaming-Responses mit Token-Counting und Format-Konvertierung
    - Provider-spezifische Features: Anthropic Prompt Caching, OpenAI Tool Calling, Gemini Extended Thinking
    - Unterstuetzte Provider: Anthropic, OpenAI, OpenRouter, Google Gemini, AWS Bedrock, Azure OpenAI, GCP Vertex, DeepSeek, Ollama, LM Studio, Cerebras, Groq + jede OpenAI-kompatible API
  - **Context Management & Token-Optimierung:**
    - Context Window Progress Bar mit Input/Output Token Tracking
    - Auto Compact bei ~80% Context-Auslastung (Conversation Summary)
    - Redundant File Read Removal (nur neueste Version im Context behalten)
    - `load_mcp_documentation` Tool statt statischer MCP-Instruktionen im System-Prompt (~8.000 Token gespart)
    - Task-Sortierung nach Cost/Token Usage
  - **State Management (Three-Tier):**
    - In-Memory Cache → 500ms Debounced Writes → VS Code APIs + Filesystem
    - Settings-Praezedenz: Remote Config (Org) > Task Settings > Global Settings
    - VS Code `secrets` API fuer API Keys (verschluesselt)
    - Mutex-Protected Concurrent Access
  - **gRPC Communication (Extension↔Webview):**
    - Protocol Buffers fuer Type-Safe Bidirectional Messaging
    - Services: StateService, TaskService, ModelsService, FileService, UiService, McpService
    - `window.postMessage()` mit Type "grpc_request" als Transport
  - **Diff-basiertes File-Editing:**
    - Search/Replace Blocks (SEARCH + REPLACE Pattern)
    - Lenient Matching (Whitespace-tolerant, Close-Match statt Fail)
    - VscodeDiffViewProvider mit Custom URI Scheme (`cline-diff`)
    - Side-by-Side Vergleich vor Approval
    - Atomic Replacements via `vscode.WorkspaceEdit`
  - **Agent Client Protocol (ACP):** JSON-RPC ueber stdin/stdout fuer Cross-Editor-Support (Zed-kompatibel)
  - **Enterprise Features:** SSO, RBAC, Audit Logs, VPC Deployments, OpenTelemetry, `.clinerules`-basierte Directory-Permissions
  - **Cost/Token Tracking:**
    - Real-Time Token/Cache/Context-Usage Anzeige
    - Task-sortierung nach Cost oder Token-Verbrauch
    - OpenRouter `usage_details` fuer praeziseres Cost-Tracking
    - Provider-Routing-Optimierung: Throughput, Price, oder Latency
  - **Build System:** esbuild (Extension) + Vite (Webview UI), Protocol Buffer Codegen via `ts-proto`
- **Staerken:**
  - Human-in-the-Loop als Architektur-Prinzip — nicht nachtraeglich aufgesetzt
  - Checkpoint System (Shadow Git) ist innovativ und verhindert Datenverlust
  - MCP-Extensibility ermoeglicht unbegrenzte Tool-Erweiterung ohne Plugin-System
  - Plan/Act-Trennung mit separaten Models spart Kosten signifikant
  - 40+ LLM-Provider nativ — mehr als jeder andere Coding-Agent
  - `.clinerules` + Workflows = deklarative Projekt-Konfiguration
  - Auto-Approve mit granularer Kontrolle pro Tool-Kategorie
  - Diff-basiertes Editing mit Side-by-Side Review
  - Zero Server-Side — keine Daten verlassen die Maschine
  - Context Management mit Auto-Compact bei ~80% Window-Auslastung
  - Aktive Community (4M+ Nutzer), regelmaessige Releases
  - ACP fuer Cross-Editor-Portabilitaet (VS Code, Zed, JetBrains)
  - Enterprise-ready (SSO, RBAC, Audit)
- **Schwaechen:**
  - Hoher Token-Verbrauch — User berichten $50/Tag bei intensiver Nutzung
  - An VS Code gebunden (CLI existiert, aber VS Code ist Primary)
  - Kein Standalone-Service — nicht als Docker-Container deploybar
  - Learning Curve fuer .clinerules + Workflows + MCP-Setup
  - Kein Multi-Projekt-Dashboard
  - Kein Roadmap/Feature-Map-Management
  - Kein eingebautes LLM-Routing/Load-Balancing (User waehlt manuell)
  - Context-Window-Limits bei sehr langen Tasks
  - Kein Agent-zu-Agent-Orchestrierung (Single-Agent-Architektur)
  - Code-Qualitaet nicht immer optimal — Post-Review noetig
  - Kein Budget-Enforcement (nur Tracking, kein Hard-Limit)
- **Relevanz fuer CodeForge:**
  - **Plan/Act Mode Pattern:** Direkte Inspiration fuer CodeForge's Plan→Approve→Execute Workflow mit separaten LLM-Konfigurationen pro Phase
  - **Checkpoint System:** Shadow-Git-Konzept als Modell fuer CodeForge's Rollback-Mechanismus (aber in Docker-Containern statt VS Code)
  - **Approval Flow (Ask/Say Pattern):** Granulares Permission-System als Referenz fuer CodeForge's Human-in-the-Loop-Design (Web-GUI statt VS Code Panel)
  - **MCP Integration:** CodeForge kann MCP-Server als Tool-Erweiterung nutzen — Standard-Protokoll, kein eigenes Plugin-System noetig
  - **`.clinerules` Pattern:** Deklarative Projekt-Konfiguration als Vorbild fuer CodeForge's YAML-basierte Projekt-Einstellungen
  - **Context Management:** Auto-Compact und Redundant-File-Read-Removal als Strategien fuer CodeForge's History Processors
  - **Diff-basiertes Editing:** Search/Replace mit Lenient Matching als Muster fuer CodeForge's File-Change-Approval in der Web-GUI
  - **Tool-Kategorisierung:** 5 Tool-Kategorien (Files, Terminal, Browser, MCP, Context) als Taxonomie fuer CodeForge's Tool-System
  - **Anti-Patterns (was CodeForge anders macht):** Single-Agent (CodeForge: Multi-Agent), VS Code gebunden (CodeForge: Standalone-Service), kein LLM-Routing (CodeForge: LiteLLM), kein Budget-Enforcement (CodeForge: harte Limits)

### Goose (Block / Square)
- **URL:** https://github.com/block/goose
- **Stars:** ~30.400 | **Lizenz:** Apache 2.0 | **Sprache:** Rust (Core), Python/JavaScript Bindings, Electron (Desktop)
- **Beschreibung:** Block's (ehemals Square) erweiterbarer AI-Agent. Geht ueber Code-Vorschlaege hinaus: installiert, fuehrt aus, editiert und testet mit beliebigem LLM. In Rust neu geschrieben fuer Portabilitaet. Tiefe MCP-Integration (1.700+ Server).
- **Kernfeatures:**
  - Rust-Core fuer Cross-Platform-Binary-Distribution und Embeddability
  - MCP-native Design (1.700+ Extensions)
  - Multi-Model-Konfiguration fuer Cost/Performance-Optimierung
  - CLI + Desktop App, 350+ Contributors, 100+ Releases in einem Jahr
  - Backed by Block (Square/Cash App/Afterpay)
- **Relevanz fuer CodeForge:** **Hoch als Backend-Agent.** MCP-natives Design, Multi-Model-Support, Apache 2.0. Rust-Core mit Python/JS-Bindings ermoeglicht programmatische Steuerung.

### OpenCode (SST)
- **URL:** https://github.com/anomalyco/opencode
- **Stars:** ~100.000+ | **Lizenz:** MIT | **Sprache:** Go (Backend/TUI), SQLite
- **Beschreibung:** Open-Source AI Coding Agent fuer das Terminal, vom SST-Team (Serverless Stack). Interaktives TUI mit Bubble Tea, Client/Server-Architektur, LSP-Integration, Session-Management.
- **Kernfeatures:**
  - Client/Server-Architektur (TUI ist nur ein Client; kann remote gesteuert werden, z.B. von Mobile App)
  - Vim-aehnlicher Editor im Terminal
  - LSP-Integration fuer sprachbewusste Completions
  - Provider-agnostisch (Claude, OpenAI, Gemini, lokale Modelle)
  - Persistente SQLite-Sessions
- **Relevanz fuer CodeForge:** **Hoch als Backend-Agent.** Client/Server-Architektur ideal fuer programmatische Steuerung. MIT-lizenziert, in Go geschrieben (gleiche Sprache wie CodeForge Core). LSP-Integration wertvolles Feature.

### Plandex
- **URL:** https://github.com/plandex-ai/plandex
- **Stars:** ~14.700 | **Lizenz:** MIT | **Sprache:** Go (CLI + Server)
- **Beschreibung:** Terminal-basierter AI Coding Agent speziell fuer grosse Projekte und realistische Tasks. Bis zu 2M Token Context, tree-sitter Project Maps fuer 20M+ Token Verzeichnisse. Cumulative Diff Review Sandbox, volle Versionskontrolle fuer AI-generierte Aenderungen, Dockerized Server.
- **Kernfeatures:**
  - Designed fuer grosse Multi-File-Tasks (2M Token Context Window)
  - Cumulative Diff Review Sandbox (Aenderungen bleiben separat bis Approval)
  - Built-in Version Control fuer AI-Plaene (Branching, Rollback)
  - Multi-Model-Support (Mix Anthropic, OpenAI, Google, Open Source)
  - Dockerized Server Mode fuer Self-Hosting
  - REPL-Modus mit Fuzzy Auto-Complete
- **Relevanz fuer CodeForge:** **Hoch als Backend-Agent.** Planning-First-Ansatz, Diff-Sandbox und Version Control fuer AI-Aenderungen passen perfekt zu CodeForge's Plan→Approve→Execute→Review→Deliver Workflow. In Go geschrieben (gleicher Stack), MIT-lizenziert, Dockerized.

### AutoCodeRover (NUS)
- **URL:** https://github.com/nus-apr/auto-code-rover
- **Paper (ISSTA 2024):** https://github.com/nus-apr/auto-code-rover/blob/main/preprint.pdf
- **Stars:** ~2.800 | **Lizenz:** GPL-3.0 | **Sprache:** Python
- **Beschreibung:** Akademisch fundierter autonomer Software-Engineer (National University of Singapore). Nutzt AST-basierte Code-Suche fuer Bug-Fixing und Feature-Addition. 46.2% auf SWE-bench Verified bei unter $0.70 pro Task.
- **Kernfeatures:**
  - Program Structure Aware: Code-Suche ueber Abstract Syntax Tree (nicht Plain-Text)
  - Statistische Fault Localization via Test-Suites
  - Extrem kosteneffizient ($0.70/Task, 7 Minuten/Task)
  - Supports GPT-4, Gemini, Claude, Llama (via Ollama)
- **Relevanz fuer CodeForge:** **Mittel als Backend-Agent.** AST-aware Search und Fault Localization sind einzigartige Faehigkeiten. Sehr niedrige Kosten attraktiv fuer automatisiertes Bug-Fixing. GPL-3.0 schraenkt Integration ein.

### Roo Code (ehemals Roo Cline)
- **URL:** https://github.com/RooCodeInc/Roo-Code
- **Stars:** ~22.200 | **Lizenz:** Apache 2.0 | **Sprache:** TypeScript (VS Code Extension)
- **Beschreibung:** AI-gestuetzter autonomer Coding-Agent als VS Code Extension. Bietet ein "ganzes Dev-Team" an AI-Agents ueber das Modes-System (QA Engineer, Product Manager, Code Reviewer, etc.). Cloud Agents via Web, Slack oder GitHub erreichbar.
- **Kernfeatures:**
  - **Modes System:** Spezialisierte Agent-Rollen (QA, PM, Architect, Reviewer) mit Tool-Restrictions pro Mode
  - **Custom Modes:** Eigene spezialisierte Agents mit Custom Prompts erstellbar
  - **Flexible Approval:** Manuell, autonom oder hybrid
  - **MCP Integration** fuer unbegrenzte Custom Tools
  - **Cloud Agents:** Work delegieren via Web, Slack oder GitHub
  - **Roomote Control:** Remote-Steuerung lokaler VS Code Tasks
- **Relevanz fuer CodeForge:** **Hoch als Pattern-Referenz UND potentielles Backend (Headless Mode).** Modes-System (spezialisierte Agent-Rollen mit eingeschraenktem Tool-Zugriff) direkt relevant fuer CodeForge's Agent-Spezialisierung. Cloud Agents als Modell fuer Multi-Interface-Vision.

### Codex CLI (OpenAI)
- **URL:** https://github.com/openai/codex
- **Stars:** ~55.000 | **Lizenz:** Apache 2.0 | **Sprache:** TypeScript (Node.js CLI)
- **Beschreibung:** OpenAI's offizieller Open-Source Coding-Agent fuer das Terminal. Liest, modifiziert und fuehrt Code lokal aus mit o3/o4-mini Modellen. Multimodale Inputs (Text, Screenshots, Diagramme), Rich Approval Workflow, Zero-Setup.
- **Kernfeatures:**
  - Offizielles OpenAI-Produkt mit nativer Model-Integration
  - Multimodal: Text, Screenshots, Diagramme als Input
  - Drei Approval-Modi (suggest, auto-edit, full-auto)
  - GitHub Action verfuegbar fuer CI/CD-Integration
  - Lokale Ausfuehrung (Code verlaesst nie die Umgebung)
  - Community Fork "open-codex" fuer Gemini, OpenRouter, Ollama
- **Relevanz fuer CodeForge:** **Hoch als Backend-Agent.** Apache 2.0, Terminal-basiert, Approval Workflow. GitHub Action als CI/CD-Integration-Pattern. Community Fork (open-codex) mit Multi-Provider-Support.

### SERA (Allen Institute for AI)
- **URL:** https://github.com/allenai/sera-cli | **Models:** https://huggingface.co/allenai/SERA-32B-GA
- **Lizenz:** Apache 2.0 (CLI + Open Model Weights) | **Sprache:** Python
- **Beschreibung:** Familie offener Coding-Agent-Modelle von Ai2 (Allen Institute for AI). SERA-32B erreicht 54.2% auf SWE-bench Verified, vergleichbar mit proprietaeren Modellen. Trainierbar fuer ~$400, anpassbar an private Codebases fuer ~$1.300.
- **Kernfeatures:**
  - Open Model Weights (8B und 32B Varianten)
  - Extrem niedrige Trainingskosten ($400 fuer Reproduktion, $1.300 fuer Private-Codebase-Anpassung)
  - "Student surpasses teacher" — kleineres Open Model uebertrifft groesseres proprietaeres Teacher-Model
  - Designed fuer Customization auf privaten Codebases
  - Nutzt Claude Code als Execution Harness
- **Relevanz fuer CodeForge:** **Hoch als Backend-Modell.** SERA-Modelle koennen via Ollama/vLLM hinter LiteLLM als selbst-gehostete Alternative zu proprietaeren APIs deployed werden. Besonders relevant fuer CodeForge's "Reine Completion"-Tier (braucht Context + Tools + Quality Layer).

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

---

## 10. Coding-Agent-Vergleich: Cline vs Devika

### Architektur-Vergleich

| Dimension | Cline | Devika |
|---|---|---|
| **Typ** | VS Code Extension (+ CLI + JetBrains) | Standalone Web-App |
| **Backend** | TypeScript (Node.js Extension Host) | Python (Flask + SocketIO) |
| **Frontend** | React (Webview in VS Code) | SvelteKit (Port 3001) |
| **Kommunikation** | gRPC / Protocol Buffers | Socket.IO (WebSocket) + REST |
| **Datenbank** | VS Code Storage + Filesystem + Secrets API | SQLite |
| **LLM-Integration** | 40+ Provider via ApiHandler Factory | Direkte API-Calls pro Provider |
| **Agent-Modell** | Single-Agent mit Tool-Invocations | Multi-Agent (9 spezialisierte Sub-Agents) |
| **Execution-Modell** | Recursive Conversation Loop mit HITL | Sequential Pipeline (Plan→Research→Code) |
| **File-Editing** | Diff-basiert (Search/Replace + Side-by-Side Review) | Direktes Schreiben auf Disk |
| **Approval-Flow** | Granular (per Tool-Kategorie, .clinerules) | Keiner |
| **Checkpointing** | Shadow Git (isoliertes Git-Repo) | Keines |
| **Browser** | Integriert (Screenshot + Interaction) | Playwright + LLM-gesteuerter Crawler |
| **MCP-Support** | Vollstaendig (Hub, Marketplace, Self-Build) | Keiner |
| **Context-Management** | Auto-Compact, Redundant-Read-Removal | Keines (voller Kontext) |
| **Prompt-Templates** | Im Code (System-Prompt-Builder) | Jinja2-Dateien pro Agent |
| **Status** | Aktiv (4M+ User, regelmaessige Releases) | Stagniert/aufgegeben (seit Mitte 2025) |
| **Lizenz** | Apache 2.0 | MIT |

### Feature-Vergleich

| Feature | Cline | Devika |
|---|---|---|
| Plan/Act Modes | Ja (separate Models konfigurierbar) | Implizit (Planner-Agent, kein User-Toggle) |
| Multi-LLM | 40+ Provider | 6 Provider (Claude, GPT, Gemini, Mistral, Groq, Ollama) |
| Local Models | Ollama + LM Studio | Ollama |
| Cost Tracking | Real-Time Token/Cost Anzeige | Basic Token Usage im Agent State |
| Budget Limits | Nein (nur Tracking) | Nein |
| Web Research | Via MCP-Server | Eingebaut (Bing/Google/DuckDuckGo + Crawler) |
| Code Execution | Terminal Commands mit Approval | Runner-Agent (Multi-OS Sandbox) |
| Git Integration | Shadow Git + Workspace Git | GitHub Clone only |
| Deployment | Nein | Netlify-Integration |
| Project Management | Nein | Basic (Project-based Organization) |
| Keyword Extraction | Nein | SentenceBERT |
| Report Generation | Nein | Reporter-Agent (PDF) |
| Enterprise | SSO, RBAC, Audit, VPC | Nein |
| Cross-Editor | ACP (VS Code, Zed, JetBrains) | Standalone (Browser-basiert) |

### Synthese: Was CodeForge von Cline uebernimmt

| Konzept | Umsetzung in CodeForge |
|---|---|
| Plan/Act Mode | Plan→Approve→Execute Workflow mit separaten LLM-Configs pro Phase |
| Shadow Git Checkpoints | Rollback-Mechanismus in Agent-Containern (Git-basiert) |
| Ask/Say Approval Pattern | Web-GUI-basierter Approval-Flow mit granularen Permissions |
| MCP als Extensibility | MCP-Server als Tool-Erweiterung (Standard-Protokoll) |
| .clinerules Pattern | YAML-basierte Projekt-Konfiguration (Agent-Verhalten, Permissions) |
| Auto-Compact Context | History Processors mit automatischer Zusammenfassung bei ~80% |
| Diff-basiertes File Review | Side-by-Side Diff-Anzeige in Web-GUI vor Approval |
| Tool-Kategorisierung (5) | Files, Terminal, Browser, MCP, Context als Tool-Taxonomie |
| Auto-Approve Granularitaet | Per-Tool-Kategorie Autonomie-Level in Projekt-Settings |
| Provider-Routing (Plan vs Act) | LiteLLM Tags (plan/default) fuer unterschiedliche Models |

### Synthese: Was CodeForge von Devika uebernimmt

| Konzept | Umsetzung in CodeForge |
|---|---|
| Sub-Agent-Architektur | Worker-Module mit Planner/Researcher/Coder-Trennung |
| Jinja2-Prompt-Templates | Prompts als separate Template-Dateien, nicht im Code |
| SentenceBERT/KeyBERT Keywords | Semantische Keyword-Extraction fuer besseres Retrieval |
| Agent State Visualization | Real-Time Dashboard (Internal Monologue, Steps, Browser, Terminal) |
| LLM-gesteuerter Web Crawler | Web-Research-Worker (Page → LLM → Action Loop) |
| Stateless Agent Design | Worker-Module stateless, State im Go Core / Message Queue |
| Sequential Pipeline | Plan→Research→Code als Basis-Workflow (erweiterbar via DAG) |
| Domain Experts | Spezialisierte Knowledge-Module pro Domaene |
| Socket.IO Real-Time | WebSocket-basierte Live-Updates (CodeForge: native WS) |

### Explizit NICHT uebernommen (Cline + Devika)

| Konzept | Grund |
|---|---|
| Cline's VS Code Binding | CodeForge ist Standalone-Service (Docker), nicht IDE-Extension |
| Cline's gRPC/Protobuf UI-Kommunikation | Web-GUI via REST + WebSocket (simpler, ausreichend) |
| Cline's Shadow Git in Extension | CodeForge nutzt Git in Agent-Containern (Docker-native) |
| Cline's Single-Agent-Architektur | CodeForge: Multi-Agent-Orchestrierung ueber Go Core |
| Cline's Provider-spezifische ApiHandler | LiteLLM als einheitlicher Proxy (kein eigenes Provider-Interface) |
| Devika's Flask Single-Process | Go Core + Python Workers via NATS/Redis |
| Devika's SQLite | PostgreSQL (Production-grade) |
| Devika's fehlender Approval-Flow | Human-in-the-Loop ist Kernprinzip von CodeForge |
| Devika's fehlende Checkpoints | Git-basiertes Checkpointing in Agent-Containern |
| Devika's direkte Provider-Calls | LiteLLM routet alles einheitlich |
| Devika's Browser als Pflicht-Dependency | Browser optional, nur fuer Web-Research-Tasks |

---

## 11. Erweiterte Konkurrenzanalyse: 12 neue Tools (Uebersicht)

### Gesamtuebersicht

| # | Name | Stars | Lizenz | Typ | Stack | CodeForge-Relevanz |
|---|------|-------|--------|-----|-------|-------------------|
| 1 | Codel | ~2.400 | AGPL-3.0 | Konkurrent | Go + React | Hoch (aehnliche Architektur) |
| 2 | AutoForge | ~1.600 | TBD | Konkurrent | Python + React | Mittel (Multi-Session Pattern) |
| 3 | bolt.diy | ~19.000 | MIT* | Konkurrent | TypeScript/Remix | Mittel (Multi-LLM App Builder) |
| 4 | Dyad | ~16.800 | Apache 2.0 | Konkurrent | TypeScript/Electron | Niedrig-Mittel (Local App Builder) |
| 5 | CAO (AWS) | ~210 | Apache 2.0 | Konkurrent | Python/tmux/MCP | Sehr hoch (Multi-Agent Orchestrator) |
| 6 | Goose | ~30.400 | Apache 2.0 | Backend | Rust | Hoch (MCP-nativer Agent) |
| 7 | OpenCode | ~100.000 | MIT | Backend | Go | Hoch (Client/Server Go Agent) |
| 8 | Plandex | ~14.700 | MIT | Backend | Go | Hoch (Planning-First Go Agent) |
| 9 | AutoCodeRover | ~2.800 | GPL-3.0 | Backend | Python | Mittel (AST-aware, GPL) |
| 10 | Roo Code | ~22.200 | Apache 2.0 | Beides | TypeScript | Hoch (Modes System, Cloud Agents) |
| 11 | Codex CLI | ~55.000 | Apache 2.0 | Backend | TypeScript | Hoch (OpenAI offiziell, GH Action) |
| 12 | SERA | Neu | Apache 2.0 | Backend-Modell | Python | Hoch (Self-Hosted Open Model) |

### Backend-Integration Prioritaeten

**Prio 1 — Go-native, MIT/Apache 2.0, hoher Community-Support:**
1. **Goose** — MCP-native, Rust mit Bindings, 30k+ Stars, Apache 2.0
2. **OpenCode** — Go-basiert (gleicher Stack), Client/Server, MIT, 100k+ Stars
3. **Plandex** — Go-basiert, Planning-First mit Diff-Sandbox, MIT, Dockerized

**Prio 2 — Starke Features, gute Lizenz:**
4. **Codex CLI** — OpenAI offiziell, Multimodal, GitHub Action, Apache 2.0
5. **Roo Code** — Modes-System, Cloud Agents, Headless-Potential, Apache 2.0

**Prio 3 — Nischen-Kandidaten:**
6. **SERA** — Open Model Weights fuer Self-Hosting (Ollama/vLLM hinter LiteLLM)
7. **AutoCodeRover** — AST-aware, $0.70/Task, aber GPL-3.0

### Naechster Konkurrent zu beobachten

**CLI Agent Orchestrator (AWS):** Obwohl aktuell klein (~210 Stars), ist es AWS-gestuetzt und nutzt das gleiche Multi-Agent-Orchestrierungs-Pattern (Supervisor/Worker, tmux/MCP, Support fuer Claude Code + Aider). Koennte schnell wachsen. Fehlt: Web-GUI, Projekt-Dashboard, Roadmap — genau CodeForge's Differenzierung.

### Synthese: Neue Patterns aus der erweiterten Analyse

| Pattern | Quelle | Anwendung in CodeForge |
|---|---|---|
| Supervisor/Worker via tmux/MCP | CAO (AWS) | Referenz fuer Agent-Session-Isolation |
| Client/Server Agent Architecture | OpenCode | Agent als Server, CodeForge Core als Client |
| Modes System (Agent-Rollen) | Roo Code | YAML-konfigurierbare Agent-Spezialisierung |
| Cumulative Diff Sandbox | Plandex | Aenderungen bleiben separat bis Approval |
| Two-Agent Pattern (Initializer+Coder) | AutoForge | Test-First Agent-Workflow |
| MCP-native Tool-Extensibility | Goose | Standard-Protokoll fuer Tool-Integration |
| Open Model Weights Deployment | SERA | Self-Hosted LLM via Ollama/vLLM hinter LiteLLM |
| GitHub Action CI/CD Pattern | Codex CLI | Agent-Ausfuehrung in CI/CD Pipelines |
| AST-based Code Search | AutoCodeRover | Ergaenzung zu tree-sitter Repo Map |
| Smart Docker Image Picker | Codel | Automatische Container-Image-Auswahl pro Task |
| Flow Scheduling (Cron) | CAO (AWS) | Unbeaufsichtigte automatische Agent-Ausfuehrung |
| Cloud Agent Delegation | Roo Code | Work delegieren via Web/Slack/GitHub |
