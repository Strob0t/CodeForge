# CodeForge — Architektur

## Ueberblick

CodeForge ist ein containerisierter Service zur Orchestrierung von AI-Coding-Agents.
Die Architektur folgt einem Drei-Schichten-Modell mit strikter Sprachtrennung nach Aufgabe.

## Systemarchitektur

```
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                     (SolidJS)                        │
│                                                     │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌────────┐ │
│  │ Projekt  │  │ Roadmap/ │  │  LLM   │  │ Agent  │ │
│  │Dashboard │  │FeatureMap│  │Provider│  │Monitor │ │
│  └─────────┘  └──────────┘  └────────┘  └────────┘ │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ HTTP/WS  │  │  Agent   │  │   Repo   │          │
│  │ Server   │  │Lifecycle │  │ Manager  │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │Scheduling│  │  Auth /  │  │  Queue   │          │
│  │ Engine   │  │ Sessions │  │ Producer │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐                        │
│  │Auto-Detect│  │ PM Sync  │                        │
│  │ Engine   │  │ Service  │                        │
│  └──────────┘  └──────────┘                        │
└────────────┬────────────────────────┬───────────────┘
             │  Message Queue         │
             │  (NATS / Redis)        │
┌────────────▼──────┐  ┌─────────────▼───────────────┐
│  Python Worker 1  │  │  Python Worker N            │
│                   │  │                             │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │  LangGraph  │  │  │  │  LangGraph  │           │
│  │  (Agents)   │  │  │  │  (Agents)   │           │
│  └─────────────┘  │  │  └─────────────┘           │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │ Agent Exec  │  │  │  │ Agent Exec  │           │
│  │(Aider, etc.)│  │  │  │(OpenHands)  │           │
│  └─────────────┘  │  │  └─────────────┘           │
└────────┬──────────┘  └──────────┬──────────────────┘
         │  OpenAI-kompatible API │
┌────────▼────────────────────────▼──────────────────┐
│            LiteLLM Proxy (Sidecar)                  │
│  127+ Provider │ Routing │ Budgets │ Cost-Tracking  │
└────────────────────────┬───────────────────────────┘
                         │ Provider APIs
┌────────────────────────▼───────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter │
└────────────────────────────────────────────────────┘
```

## Schichten im Detail

### Frontend (TypeScript)

- **Zweck:** Web-GUI fuer alle Nutzerinteraktionen
- **Kommunikation:** REST-API fuer CRUD, WebSocket fuer Echtzeit-Updates (Agent-Logs, Status)
- **Kernmodule:**
  - Projekt-Dashboard (Repos verwalten, Status-Uebersicht)
  - Roadmap/Feature-Map Editor (visuell, OpenSpec-kompatibel)
  - LLM-Provider-Management (Konfiguration, Kosten-Tracking)
  - Agent-Monitor (Live-Logs, Task-Status, Ergebnisse)

### Core Service (Go)

- **Zweck:** Performantes Backend fuer HTTP, WebSocket, Scheduling und Koordination
- **Warum Go:** Native Concurrency (Goroutines), minimaler RAM (~10-20MB), schnelle Startzeiten, exzellent fuer tausende gleichzeitige Connections
- **Kernmodule:**
  - HTTP/WebSocket Server
  - Agent Lifecycle Management (Start, Stop, Status, Restart)
  - Repo Manager (Git, GitHub, GitLab, SVN Integration)
  - Scheduling Engine (Task-Queue, Priorisierung)
  - Auth / Sessions / Multi-Tenancy
  - Queue Producer (Jobs an Python Worker dispatchen)

### AI Workers (Python)

- **Zweck:** LLM-Interaktion und Agent-Ausfuehrung
- **Warum Python:** Nativer Zugang zum AI-Ecosystem (LiteLLM, LangGraph, alle LLM-SDKs)
- **Skalierung:** Horizontal ueber Message Queue — beliebig viele Worker-Instanzen
- **Kernmodule:**
  - LiteLLM Integration (Multi-Provider-Routing: OpenAI, Claude, Ollama, etc.)
  - Agent Execution (Aider, OpenHands, SWE-agent als austauschbare Backends)
  - LangGraph Orchestrierung (fuer komplexe Multi-Agent-Workflows)

## Kommunikation zwischen Schichten

| Von → Nach | Protokoll | Zweck |
|---|---|---|
| Frontend → Go | REST (HTTP/2) | CRUD Operationen |
| Frontend → Go | WebSocket | Echtzeit-Updates, Logs |
| Go → Python Workers | Message Queue (NATS/Redis) | Job-Dispatch |
| Python Workers → Go | Message Queue (NATS/Redis) | Ergebnisse, Status-Updates |
| Go → LiteLLM Proxy | HTTP (OpenAI-Format) | Config-Management, Health-Checks |
| Python Workers → LiteLLM Proxy | HTTP (OpenAI-Format) | LLM-Calls (`litellm.completion()`) |
| LiteLLM Proxy → LLM APIs | HTTPS | Provider-spezifische API-Calls |
| Go → SCM (Git/SVN) | CLI / REST API | Repo-Operationen |
| Go → Ollama/LM Studio | HTTP | Local Model Auto-Discovery |
| Go → PM-Plattformen | REST API / Webhooks | Bidirektionaler PM-Sync (Plane, OpenProject, etc.) |
| Go → Repo Specs | Filesystem | Spec-Detection und -Sync (OpenSpec, Spec Kit, Autospec) |

## Design-Entscheidungen

### Warum nicht alles in Python?
Go handelt bei gleicher Last einen Bruchteil der Ressourcen. Ein Go HTTP-Server skaliert problemlos auf zehntausende gleichzeitige Connections — in Python braucht man dafuer deutlich mehr Tuning und Instanzen.

### Warum nicht alles in Go?
Das gesamte AI/Agent-Ecosystem (LiteLLM, LangGraph, Aider, OpenHands, SWE-agent, alle LLM-SDKs) ist Python. Alles ueber Bridges anbinden waere mehr Overhead als dedizierte Python-Worker.

### Warum Message Queue statt direkter Aufrufe?
- Entkopplung: Go-Service muss nicht auf langsame LLM-Calls warten
- Skalierung: Worker horizontal skalierbar
- Resilienz: Jobs gehen bei Worker-Absturz nicht verloren
- Backpressure: Queue puffert bei Last-Spitzen

### Warum YAML als einheitliches Konfigurationsformat?

**Alle Konfigurationsdateien in CodeForge verwenden YAML** — keine Ausnahme:

- Agent-Modes und Spezialisierungen
- Tool-Bundles und Tool-Definitionen
- Projekt-Einstellungen und Safety-Rules
- Autonomie-Konfiguration
- LiteLLM Config (nativ YAML)
- Prompt-Metadaten (Jinja2-Templates selbst bleiben `.jinja2`)

**Grund:** YAML unterstuetzt Kommentare. Das ist entscheidend fuer:
- Dokumentation direkt in der Config (`# Warum dieses Budget-Limit?`)
- Temporaeres Deaktivieren von Einstellungen (`# tools: [terminal]`)
- Onboarding: Contributors verstehen Configs ohne externe Doku
- Versionierung: Kommentare erklaeren Aenderungen im Git-Diff

JSON wird **nicht** fuer Konfigurationsdateien verwendet. JSON bleibt
fuer API-Responses, Event-Serialisierung und internen Datenaustausch.

## Software-Architektur: Hexagonal + Provider Registry

### Grundprinzip: Hexagonal Architecture (Ports & Adapters)

Die Kernlogik (Domain + Services) ist vollstaendig von externen Systemen isoliert.
Alle Abhaengigkeiten zeigen nach innen — nie nach aussen.

```
┌──────────────────────────────────────────────────────────┐
│                    ADAPTER (aussen)                       │
│  HTTP-Handler, GitHub, Postgres, NATS, Ollama, Aider     │
│                                                          │
│    ┌──────────────────────────────────────────────┐       │
│    │              PORTS (Grenze)                  │       │
│    │    Go Interfaces — definieren WAS die        │       │
│    │    Kernlogik braucht, nicht WIE              │       │
│    │                                              │       │
│    │    ┌──────────────────────────────┐          │       │
│    │    │        DOMAIN (Kern)         │          │       │
│    │    │   Business-Logik, Entities   │          │       │
│    │    │   Regeln, Validierung        │          │       │
│    │    │   Null externe Imports       │          │       │
│    │    └──────────────────────────────┘          │       │
│    └──────────────────────────────────────────────┘       │
└──────────────────────────────────────────────────────────┘
```

### Provider Registry Pattern

Fuer Open-Source-Erweiterbarkeit nutzt CodeForge ein Self-Registering-Provider-Pattern.
Neue Implementierungen (z.B. ein Gitea-Adapter) erfordern:

1. Ein Go-Package, das das entsprechende Interface erfuellt
2. Einen Blank-Import in `cmd/codeforge/providers.go`
3. Keine Aenderungen an der Kernlogik

#### Ablauf

```
1. Port definiert Interface + Registry
   (Register, New, Available)

2. Adapter implementiert Interface
   und registriert sich via init()

3. Blank-Import in providers.go
   aktiviert den Adapter

4. Kernlogik nutzt nur das Interface —
   weiss nicht, welcher Adapter dahinter steckt
```

Dieses Pattern folgt dem Go-Standardmuster (`database/sql` + `_ "github.com/lib/pq"`).

#### Provider-Typen

| Port | Interface | Beispiel-Adapter |
|---|---|---|
| `gitprovider` | `Provider` | github, gitlab, gitlocal, svn, gitea |
| `llmprovider` | `Provider` | openai, claude, ollama, lmstudio |
| `agentbackend` | `Backend` | aider, openhands, sweagent |
| `specprovider` | `SpecProvider` | openspec, speckit, autospec |
| `pmprovider` | `PMProvider` | plane, openproject, github_pm, gitlab_pm |
| `database` | `Store` | postgres, sqlite |
| `messagequeue` | `Queue` | nats, redis |

#### Capabilities

Nicht jeder Provider unterstuetzt alle Operationen. Statt leere Implementierungen
deklariert jeder Provider seine Faehigkeiten:

```go
type Capability string
const (
    CapClone    Capability = "clone"
    CapWebhooks Capability = "webhooks"
    CapPRs      Capability = "pull_requests"
    // ...
)
```

Die Kernlogik und das Frontend pruefen Capabilities und passen ihr Verhalten an.
SVN unterstuetzt z.B. keine Webhooks — das ist kein Fehler, sondern deklariertes Verhalten.

#### Compliance-Tests

Jeder Provider-Typ liefert eine wiederverwendbare Test-Suite (`RunComplianceTests`).
Ein neuer Adapter ruft diese Funktion auf und erhaelt automatisch alle Interface-Tests.
Contributors schreiben minimalen Test-Code und bekommen maximale Abdeckung.

### Verzeichnisstruktur Go Core

```
cmd/
  codeforge/
    main.go              # Einstiegspunkt, Dependency Injection
    providers.go         # Blank-Imports aller aktiven Adapter
internal/
  domain/                # Kern: Entities, Business Rules (null externe Imports)
    project/
    agent/
    roadmap/
  port/                  # Interfaces + Registries
    gitprovider/
      provider.go        # Interface + Capability-Definitionen
      registry.go        # Register(), New(), Available()
      compliance_test.go # Wiederverwendbare Test-Suite
    llmprovider/
    agentbackend/
    specprovider/
      provider.go        # SpecProvider Interface (Detect, ReadSpecs, WriteChange, Watch)
      registry.go        # Register(), New(), Available()
    pmprovider/
      provider.go        # PMProvider Interface (Detect, SyncItems, CreateItem, Webhooks)
      registry.go        # Register(), New(), Available()
    database/
    messagequeue/
  adapter/               # Konkrete Implementierungen
    github/
    gitlab/
    gitlocal/
    svn/
    openai/
    ollama/
    aider/
    openhands/
    openspec/            # OpenSpec Spec-Adapter
    speckit/             # GitHub Spec Kit Adapter
    autospec/            # Autospec Adapter
    plane/               # Plane.so PM-Adapter
    openproject/         # OpenProject PM-Adapter
    github_pm/           # GitHub Issues/Projects PM-Adapter
    gitlab_pm/           # GitLab Issues/Boards PM-Adapter
    postgres/
    sqlite/
    nats/
    redis/
  service/               # Use Cases (verbindet Domain mit Ports)
```

## LLM Capability Levels

Nicht jedes LLM bringt dieselben Faehigkeiten mit. CodeForge muss die Luecken
schliessen, damit auch einfache Models produktiv eingesetzt werden koennen.

### Das Problem

```
Claude Code / Aider       →  eigene Tool-Usage, Codebase-Search, Agent-Loop
OpenAI API (direkt)       →  Function Calling, aber kein Codebase-Kontext
Ollama (lokal)            →  reines Text-Completion, keine Tools, kein Kontext
```

Ein lokales Ollama-Modell weiss nichts ueber das Repo, kann keine Dateien lesen
und hat kein Gedaechtnis. CodeForge muss diese Faehigkeiten bereitstellen.

### Capability-Stacking durch Python Workers

Die Workers ergaenzen fehlende Faehigkeiten je nach LLM-Level:

```
┌──────────────────────────────────────────────────────┐
│                    CodeForge Worker                    │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Context Layer (fuer alle LLMs)                │  │
│  │  GraphRAG: Vector-Search + Graph-DB +          │  │
│  │  Web-Fallback → relevanten Code/Docs finden    │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Quality Layer (optional, konfigurierbar)      │  │
│  │  Multi-Agent Debate: Pro/Con/Moderator →       │  │
│  │  Halluzinationen reduzieren, Loesungen pruefen │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Routing Layer                                 │  │
│  │  Task-basiertes Model-Routing via LiteLLM →    │  │
│  │  richtige Aufgabe an richtiges Modell          │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Execution Layer                               │  │
│  │  Agent-Backends: Aider, OpenHands, SWE-agent,  │  │
│  │  oder direkte LLM-API-Calls                    │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

### Drei LLM-Integrationsstufen

| Stufe | Beispiel | Was CodeForge bereitstellt |
|---|---|---|
| **Vollwertige Agents** | Claude Code, Aider, OpenHands | Nur Orchestrierung — Agent bringt eigene Tools mit |
| **API mit Tool-Support** | OpenAI, Claude API, Gemini | Context Layer (GraphRAG) + Routing + Tool-Definitionen |
| **Reine Completion** | Ollama, LM Studio (lokale Models) | Alles: Context, Tools, Prompt-Engineering, Quality Layer |

Je weniger ein LLM kann, desto mehr uebernimmt der CodeForge Worker.

### Worker-Module im Detail

**Context Layer — GraphRAG**
- Vector-Search (Qdrant/ChromaDB): Semantische Suche im Codebase-Index
- Graph-DB (Neo4j/optional): Beziehungen zwischen Code-Elementen (Imports, Calls, Vererbung)
- Web-Fallback (Tavily/SearXNG): Dokumentation und Stack Overflow bei fehlender lokaler Info
- Ergebnis: Relevanter Kontext wird dem LLM-Prompt vorangestellt

**Quality Layer — Mehrstufige Qualitaetssicherung**

Drei Strategien, abgestuft nach Aufwand und Kritikalitaet:

1. **Action Sampling** (leichtgewichtig)
   - Mehrere unabhaengige LLM-Antworten generieren
   - AskColleagues: N Vorschlaege, LLM synthetisiert die beste Loesung
   - BinaryComparison: Paarweiser Vergleich, Gewinner wird ausgewaehlt
   - Fuer alltaegliche Tasks mit moderatem Qualitaetsanspruch

2. **RetryAgent + Reviewer** (mittel)
   - Agent loest Task mehrfach (Environment-Reset zwischen Versuchen)
   - LLM-basierter Reviewer bewertet jede Loesung:
     - Score-Modus: Numerische Bewertung, Durchschnitt ueber Samples
     - Chooser-Modus: Direkter Vergleich aller Loesungen
   - Beste Loesung wird ausgewaehlt
   - Fuer wichtige Changes mit messbarer Qualitaet

3. **Multi-Agent Debate** (schwergewichtig)
   - Pro-Agent argumentiert fuer eine Loesung
   - Con-Agent sucht Schwachstellen
   - Moderator synthetisiert das Ergebnis
   - Fuer kritische Architektur-Entscheidungen und sicherheitsrelevante Changes

Alle drei Strategien sind optional und konfigurierbar per Projekt/Task.

**Routing Layer — Intelligentes Model-Routing**
- Task-Klassifikation: Architektur, Code-Generierung, Review, Docs, Tests
- Kosten-Optimierung: Einfache Tasks an guenstige Models, komplexe an starke
- Latenz-Routing: Schnelle Antworten fuer interaktive Nutzung
- Fallback-Ketten: Wenn ein Provider ausfaellt, automatisch naechsten nutzen
- Routing-Regeln konfigurierbar per Projekt und per User
- **Kosten-Management:**
  - Budget-Limits pro Task, pro Projekt, pro User
  - Automatisches Cost-Tracking ueber LiteLLM
  - Warnung/Stopp bei Budget-Ueberschreitung
  - API-Call-Limits pro Agent-Run

## Agent Execution: Modes, Safety, Workflow

### Drei Execution Modes

Nicht jeder Anwendungsfall braucht eine Sandbox. CodeForge unterstuetzt drei Modi:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Execution Modes                             │
│                                                                 │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │   Sandbox    │  │     Mount        │  │     Hybrid       │  │
│  │              │  │                  │  │                  │  │
│  │  Isolierter  │  │  Agent arbeitet  │  │  Sandbox mit     │  │
│  │  Container,  │  │  direkt auf      │  │  gemounteten     │  │
│  │  Repo-Kopie  │  │  gemountem Pfad  │  │  Volumes         │  │
│  │  im Container│  │  des Hosts       │  │  (read/write     │  │
│  │              │  │                  │  │   konfigurierbar)│  │
│  └──────────────┘  └──────────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

| Modus | Wann | Sicherheit | Geschwindigkeit |
|---|---|---|---|
| **Sandbox** | Untrusted Agents, fremde Models, Batch-Jobs | Hoch — kein Zugriff auf Host | Mittel — Container-Overhead, Repo-Copy |
| **Mount** | Trusted Agents (Claude Code, Aider), lokale Entwicklung | Niedrig — direkter Dateizugriff | Hoch — kein Overhead |
| **Hybrid** | Review-Workflows, CI-artige Ausfuehrung | Mittel — kontrollierter Zugriff | Mittel |

**Mount-Modus im Detail:**
- Agent erhaelt Pfad zum gemounteten Repo (z.B. `/workspace/my-project`)
- Aenderungen landen direkt im Dateisystem des Hosts
- Ideal fuer interaktive Nutzung: User sieht Aenderungen sofort in seiner IDE
- Kein Container noetig — Agent laeuft im Worker-Prozess oder nativem Tool

**Sandbox-Modus im Detail:**
- Docker-Container pro Task (Docker-in-Docker)
- Repo wird in den Container kopiert oder als read-only Volume gemountet
- Agent bekommt alle noetigen Tools im Container bereitgestellt
- Ergebnis wird als Patch/Diff extrahiert und auf das Original-Repo angewendet

**Hybrid-Modus im Detail:**
- Container mit gemountem Volume
- Mount-Rechte konfigurierbar: read-only Source + write Workspace-Copy
- Agent kann lesen, aber Aenderungen gehen in eine Kopie
- User reviewed und merged manuell

### Tool-Provisioning fuer Sandbox-Agents

Agents in Sandbox-Containern brauchen die richtigen Tools. CodeForge stellt
diese automatisch bereit — abhaengig vom Agent-Typ und Execution Mode:

```
┌─────────────────────────────────────────────────┐
│            Sandbox Container                     │
│                                                 │
│  ┌───────────────────────────────────────────┐  │
│  │  Base Image (Python/Node/Go)              │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  CodeForge Tool Layer                     │  │
│  │  - Shell (mit Safety Evaluator)           │  │
│  │  - File Read/Write/Patch                  │  │
│  │  - Grep/Search                            │  │
│  │  - Git Operations                         │  │
│  │  - Dependency Installation                │  │
│  │  - Test Runner                            │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  Repo (kopiert oder gemountet)            │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

Tools werden als Pydantic-Schema definiert und dem LLM als Function Calls
oder Tool-Definitionen uebergeben. Vollwertige Agents (Aider, OpenHands)
bringen eigene Tools mit und brauchen nur das Repo.

### Command Safety Evaluator

Jeder Shell-Befehl eines Agents durchlaeuft einen Safety-Check:

- **Destruktive Operationen** erkennen (`rm -rf`, `git push --force`, etc.)
- **Prompt Injection** in Befehlen erkennen
- **Risiko-Level** bewerten: low / medium / high
- **Tool-Blocklists:** Interaktive Programme (`vim`, `nano`), standalone
  Interpreter (`python` ohne Script), gefaehrliche Befehle — konfigurierbar
  per Projekt als YAML
- **Konfigurierbar** per Projekt: Was darf ein Agent, was nicht?
- Bei Unsicherheit: Befehl blockieren und User fragen (Human-in-the-Loop)

Fuer trusted Agents im Mount-Modus optional. Fuer lokale Models in der
Sandbox obligatorisch.

### Agent-Workflow: Plan → Execute → Review

Standardisierter Workflow fuer alle Agents — mit konfigurierbarem Autonomie-Level:

```
1. PLAN      Agent analysiert Task + Codebase, erstellt strukturierten Plan
                ↓
2. APPROVE   Plan wird zur Freigabe vorgelegt (je nach Autonomie-Level)
                ↓  (User, Safety-Rules oder Auto-Approve)
3. EXECUTE   Agent arbeitet Plan Punkt fuer Punkt ab
                ↓
4. REVIEW    Self-Review, zweiter Agent, oder Guardrail Agent
                ↓
5. DELIVER   Ergebnis als Diff/Patch, PR, oder direkte Dateiaenderung
```

- Jeder Schritt ist einzeln konfigurierbar (Skip, Auto-Approve, etc.)
- Autonomie-Level bestimmt, wer approven darf (User vs. Safety-Rules)
- Bei Level 4-5 ersetzen Safety-Rules den menschlichen Approver

### Autonomie-Spektrum

CodeForge unterstuetzt fuenf Autonomie-Level — vom voll-supervisierten
Betrieb bis zur komplett autonomen Ausfuehrung ohne User-Interaktion:

```
Level 1   Level 2     Level 3     Level 4      Level 5
supervised  semi-auto   auto-edit   full-auto    headless
  │           │           │           │            │
  ▼           ▼           ▼           ▼            ▼
 User       User        User       Safety       Safety
 approves   approves    approves   Rules        Rules
 ALLES      kritische   Terminal/  ersetzen     ersetzen
            Aktionen    Deploy     User         User
                                                + kein UI
```

| Level | Name | Wer approved | Use Case |
|---|---|---|---|
| 1 | `supervised` | User bei jedem Schritt | Lernen, kritische Codebases, Onboarding |
| 2 | `semi-auto` | User bei destruktiven Aktionen (delete, terminal, deploy) | Alltaegliche Entwicklung mit Sicherheitsnetz |
| 3 | `auto-edit` | User nur bei Terminal/Deploy, Datei-Aenderungen auto-approved | Erfahrene Nutzer, vertrauenswuerdige Agents |
| 4 | `full-auto` | Safety-Rules (Budget, Blocklists, Tests) | Batch-Jobs, trusted Agents, delegierte Tasks |
| 5 | `headless` | Safety-Rules, kein UI noetig | CI/CD, Cron-Jobs, API-getriebene Pipelines |

#### Konfiguration (YAML)

```yaml
# Projekt-Level: codeforge-project.yaml
autonomy:
  default_level: semi-auto       # Standard fuer neue Tasks

  # Safety-Rules — ersetzen den User als Guardrail bei Level 4-5
  safety:
    budget_hard_limit: 50.00     # USD — Agent stoppt bei Ueberschreitung
    max_steps: 100               # Max Aktionen pro Task
    max_file_changes: 50         # Max geaenderte Dateien pro Task
    blocked_paths:               # Dateien die nie geaendert werden duerfen
      - ".env"
      - "secrets/"
      - "**/credentials.*"
      - "production.yml"
    blocked_commands:             # Shell-Befehle die nie ausgefuehrt werden
      - "rm -rf /"
      - "DROP TABLE"
      - "git push --force"
      - "chmod 777"
    require_tests_pass: true     # Agent muss Tests gruen haben vor Deliver
    require_lint_pass: true      # Linting muss bestehen vor Deliver
    rollback_on_failure: true    # Auto-Rollback bei Test/Lint-Failure
    branch_isolation: true       # Autonome Agents arbeiten nie auf main/master
    max_cost_per_step: 2.00      # USD — einzelner LLM-Call darf max X kosten
```

#### Sicherheit bei vollautonomer Ausfuehrung

Bei Level 4 (`full-auto`) und Level 5 (`headless`) ersetzen folgende
Mechanismen den menschlichen Approver:

```
┌─────────────────────────────────────────────────────────────┐
│                  Safety Layer (ersetzt User)                  │
│                                                             │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Budget-Limiter  │  │ Command Safety  │                  │
│  │  Hard Stop bei   │  │ Evaluator       │                  │
│  │  Ueberschreitung │  │ Blocklist +     │                  │
│  │                  │  │ Regex-Matching  │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Branch-Isolation│  │ Test/Lint Gate  │                  │
│  │  Nie auf main,   │  │ Deliver nur     │                  │
│  │  immer Feature-  │  │ wenn Tests +    │                  │
│  │  Branch          │  │ Lint bestehen   │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Max-Steps       │  │ Rollback        │                  │
│  │  Endlos-Loop-    │  │ Automatisch bei │                  │
│  │  Erkennung       │  │ Failure         │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Path-Blocklist  │  │ Stall-Detection │                  │
│  │  Sensible Files  │  │ Re-Planning     │                  │
│  │  geschuetzt      │  │ oder Abbruch    │                  │
│  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

#### Headless-Modus (Level 5) — Use Cases

```yaml
# Naechtlicher Code-Review (Cron-Job)
# codeforge-schedules.yaml
schedules:
  - name: nightly-review
    cron: "0 2 * * *"                  # Jede Nacht um 2:00
    mode: reviewer
    autonomy: headless
    targets:
      - repo: "myorg/backend"
        branch: "develop"
    deliver: github-pr-comment         # Ergebnis als PR-Kommentar

  # Woechentliches Dependency-Update
  - name: weekly-deps
    cron: "0 8 * * 1"                  # Montags 8:00
    mode: dependency-updater
    autonomy: headless
    targets:
      - repo: "myorg/backend"
      - repo: "myorg/frontend"
    deliver: pull-request               # Ergebnis als neuer PR
    safety:
      require_tests_pass: true
      max_file_changes: 5

  # Webhook-getriggert: Lint-Fix bei neuem PR
  - name: auto-lint-fix
    trigger: github-webhook             # Bei neuem PR
    event: pull_request.opened
    mode: lint-fixer
    autonomy: full-auto
    deliver: push-to-branch             # Direkt auf den PR-Branch pushen
    safety:
      max_file_changes: 20
      require_lint_pass: true
```

#### API-getriebene autonome Ausfuehrung

Fuer CI/CD und externe Systeme:

```
POST /api/v1/tasks
{
  "repo": "myorg/backend",
  "task": "Fix all lint errors in src/",
  "mode": "lint-fixer",
  "autonomy": "full-auto",
  "deliver": "pull-request",
  "safety": {
    "budget_hard_limit": 10.00,
    "require_lint_pass": true
  },
  "callback_url": "https://ci.example.com/webhook"
}
```

- Keine UI-Interaktion noetig
- Ergebnis wird per Callback oder Polling abgeholt
- Ideal fuer GitHub Actions, GitLab CI, Jenkins, etc.

### Jinja2-Prompt-Templates

Alle Prompts fuer LLM-Aufrufe werden als Jinja2-Templates in separaten
Dateien gespeichert, nicht im Python-Code:

```
workers/codeforge/templates/
  planner.jinja2          # Planungs-Prompt
  coder.jinja2            # Code-Generierungs-Prompt
  reviewer.jinja2         # Review-Prompt
  researcher.jinja2       # Research-Prompt
  safety_evaluator.jinja2 # Safety-Check-Prompt
```

Vorteile:
- Prompts sind ohne Code-Aenderung anpassbar
- Contributors koennen Prompts verbessern ohne Python zu kennen
- Verschiedene Prompt-Sets fuer verschiedene LLMs moeglich
- Versionierbar und vergleichbar (Git-Diff auf Prompt-Aenderungen)

### Keyword-Extraction (KeyBERT)

Fuer den Context Layer: Semantische Keyword-Extraktion aus Tasks und Code
mittels SentenceTransformers/BERT:

- Extrahiert relevante Keywords aus User-Anfragen und Codebase
- Maximal Marginal Relevance (MMR) fuer diverse, nicht-redundante Keywords
- Keywords verbessern die Retrieval-Qualitaet im GraphRAG-Layer
- Leichtgewichtig, laeuft lokal ohne externe API

### Real-time State via WebSocket

Jede State-Mutation eines Agents wird sofort ueber WebSocket an das
Frontend emittiert:

- Agent-Status (aktiv, wartend, fertig)
- Internal Monologue (was der Agent "denkt")
- Aktueller Schritt im Workflow
- Token-Usage und Kosten in Echtzeit
- Terminal/Browser-Session-Daten

Das Frontend kann so Live-Updates darstellen ohne Polling.

### Agent-Spezialisierung: Modes System

Inspiriert von Roo Code's Modes und Cline's `.clinerules`. Statt eines
General-Purpose-Agents definiert CodeForge spezialisierte Agent-Modes
als YAML-Konfigurationen. Jeder Mode hat eigene Tools, LLM-Einstellungen
und Autonomie-Level.

#### Architektur

```
┌──────────────────────────────────────────────────────────┐
│                    Mode Registry                          │
│                                                          │
│  Built-in Modes        Custom Modes (User-definiert)     │
│  ┌──────────────┐      ┌──────────────────────────┐     │
│  │ architect    │      │ my-react-reviewer        │     │
│  │ coder        │      │ security-auditor         │     │
│  │ reviewer     │      │ docs-writer              │     │
│  │ researcher   │      │ dependency-updater       │     │
│  │ tester       │      │ ...                      │     │
│  │ lint-fixer   │      │                          │     │
│  │ planner      │      │ (YAML in Projekt oder    │     │
│  │ debugger     │      │  globaler Config)        │     │
│  └──────────────┘      └──────────────────────────┘     │
└──────────────────────────────────────────────────────────┘
```

#### Built-in Mode Definitionen

```yaml
# modes/architect.yaml
name: architect
description: "Analysiert Codebase-Struktur, plant Aenderungen, erstellt Design-Dokumente"
llm_scenario: think            # LiteLLM Tag → starkes Reasoning-Modell
autonomy: supervised           # Architektur-Entscheidungen immer mit User
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - plan                       # Strukturierten Plan erstellen
  - web_search                 # Dokumentation recherchieren
# Kein write_file, kein terminal — Architect darf nur lesen und planen
prompt_template: architect.jinja2
max_steps: 30
```

```yaml
# modes/coder.yaml
name: coder
description: "Implementiert Features, fixt Bugs, schreibt Code"
llm_scenario: default          # LiteLLM Tag → Standard-Coding-Modell
autonomy: auto-edit            # Datei-Aenderungen auto, Terminal braucht Approval
tools:
  - read_file
  - write_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # Shell-Befehle (mit Safety Evaluator)
  - git_diff
  - git_commit
  - lint
  - test
prompt_template: coder.jinja2
max_steps: 50
```

```yaml
# modes/reviewer.yaml
name: reviewer
description: "Prueft Code-Aenderungen auf Qualitaet, Bugs, Security"
llm_scenario: review           # LiteLLM Tag → Review-optimiertes Modell
autonomy: headless             # Kann komplett autonom laufen (readonly)
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
# Kein write_file — Reviewer darf nicht editieren, nur bewerten
prompt_template: reviewer.jinja2
max_steps: 30
deliver: comment               # Ergebnis als Kommentar (PR, Issue, Web-GUI)
```

```yaml
# modes/debugger.yaml
name: debugger
description: "Analysiert Fehler, reproduziert Bugs, findet Root Causes"
llm_scenario: think            # Komplexes Reasoning fuer Debugging
autonomy: semi-auto            # Terminal-Ausfuehrung mit Approval
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # Fuer Reproduktion und Tests
  - git_log
  - git_diff
  - test
  - lint
prompt_template: debugger.jinja2
max_steps: 40
```

```yaml
# modes/nightly-reviewer.yaml
name: nightly-reviewer
description: "Automatischer naechtlicher Code-Review"
llm_scenario: review
autonomy: headless             # Komplett autonom, kein UI
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
prompt_template: reviewer.jinja2
schedule: "0 2 * * *"         # Jede Nacht um 2:00
deliver: github-pr-comment
safety:
  budget_hard_limit: 5.00
  max_steps: 30
```

#### Custom Modes (User-definiert)

User koennen eigene Modes als YAML-Dateien erstellen:

```yaml
# .codeforge/modes/security-auditor.yaml
name: security-auditor
description: "Prueft Code auf OWASP Top 10, Injection, XSS, etc."
llm_scenario: think
autonomy: headless
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - lint
  - terminal                   # Fuer Security-Scanner (npm audit, bandit, etc.)
prompt_template: security-auditor.jinja2
safety:
  blocked_commands:
    - "curl"                   # Kein Netzwerk-Zugriff
    - "wget"
  max_steps: 50
deliver: security-report       # Strukturierter Security-Report
```

#### Mode-Auswahl und -Komposition

Modes koennen einzeln oder als Pipeline genutzt werden:

```yaml
# Einzelner Mode
task:
  mode: coder
  prompt: "Implementiere Feature X"

# Pipeline: Architect plant, Coder implementiert, Reviewer prueft
task:
  pipeline:
    - mode: architect
      prompt: "Analysiere die Codebase und erstelle einen Plan fuer Feature X"
    - mode: coder
      prompt: "Implementiere den Plan aus dem vorherigen Schritt"
    - mode: reviewer
      prompt: "Reviewe die Aenderungen des Coders"
    - mode: tester
      prompt: "Schreibe Tests fuer die neuen Aenderungen"

# DAG: Parallele Ausfuehrung + Abhaengigkeiten
task:
  dag:
    plan:
      mode: architect
    implement:
      mode: coder
      depends_on: [plan]
    test:
      mode: tester
      depends_on: [implement]
    review:
      mode: reviewer
      depends_on: [implement]     # Parallel zu test
    deliver:
      mode: coder
      depends_on: [test, review]  # Erst wenn beides fertig
```

#### Verzeichnisstruktur

```
# Global (shipped with CodeForge)
modes/
  architect.yaml
  coder.yaml
  reviewer.yaml
  researcher.yaml
  tester.yaml
  lint-fixer.yaml
  planner.yaml
  debugger.yaml
  dependency-updater.yaml

# Projekt-spezifisch (User-definiert)
.codeforge/
  modes/
    security-auditor.yaml
    my-react-reviewer.yaml
  project.yaml                 # Projekt-Einstellungen (Autonomie, Safety, etc.)
  schedules.yaml               # Cron-Jobs fuer autonome Tasks
```

### YAML-basierte Tool-Definitionen

Tools fuer Agents werden deklarativ in YAML definiert, nicht im Code hardcoded.
Contributors koennen neue Tools hinzufuegen, ohne Python-Code zu schreiben:

```yaml
# tools/bundles/file_ops/config.yaml
tools:
  read_file:
    docstring: "Read contents of a file"
    arguments:
      - name: path
        type: string
        required: true
        description: "Absolute path to the file"
  write_file:
    docstring: "Write contents to a file"
    arguments:
      - name: path
        type: string
        required: true
      - name: content
        type: string
        required: true
```

- Tool-Bundles sind Verzeichnisse mit `config.yaml` + optionalem Install-Script
- Automatische Konvertierung in OpenAI Function-Calling-Format
- Funktioniert mit jedem LLM, das Function Calling unterstuetzt
- Fuer LLMs ohne Function Calling: Backtick/JSON-basiertes Parsing als Fallback

### History Processors (Context-Window-Management)

Lange Agent-Sessions sprengen das Context-Window. History Processors
optimieren den Kontext als konfigurierbare Pipeline:

| Processor | Funktion |
|---|---|
| **LastNObservations** | Alte Tool-Outputs durch Zusammenfassungen ersetzen |
| **ClosedWindowProcessor** | Veraltete Datei-Views entfernen, nur aktuellsten behalten |
| **CacheControlProcessor** | Cache-Marker fuer Prompt-Caching setzen (Anthropic, etc.) |
| **RemoveRegex** | Bestimmte Patterns aus der History entfernen |

Processors werden als Pipeline nacheinander angewandt. Konfigurierbar
per Agent-Typ und LLM (kleine lokale Models brauchen aggressiveres Trimming).

### Hook-System (Observer-Pattern)

Erweiterungspunkte an Agent- und Environment-Lifecycle, ohne Core-Aenderung:

```
Agent Hooks:
  on_run_start       → Monitoring, Logging starten
  on_step_done       → Schritt aufzeichnen, Metriken aktualisieren
  on_model_query     → Kosten tracken, Rate-Limiting
  on_run_end         → Zusammenfassung, Cleanup

Environment Hooks:
  on_init            → Container vorbereiten
  on_copy_repo       → Repo-Indexierung starten
  on_startup         → Tools installieren
  on_close           → Container aufraumen
```

Hooks ermoeglichen Monitoring, Custom-Logging, Metriken-Sammlung und
Integration mit externen Systemen — alles ohne Aenderung der Kernlogik.

### Trajectory Recording und Replay

Jeder Agent-Run wird als Trajectory aufgezeichnet:

- Jeder Schritt: Thought → Action → Observation → Zeitstempel → Kosten
- Gespeichert als JSON zur Analyse und Reproduzierbarkeit
- **Replay-Modus:** Trajectory deterministisch wiederholen (Debugging)
- **Inspector:** Web-basierter Viewer in der GUI integriert
- **Batch-Statistiken:** Erfolgsraten, Kosten, Schritte ueber viele Runs

Trajectories ermoeglichen:
- Debugging fehlgeschlagener Agent-Runs
- Vergleich verschiedener LLMs/Configs auf denselben Tasks
- Audit-Trail fuer Code-Aenderungen durch Agents

### Verzeichnisstruktur Python Workers

```
workers/
  codeforge/
    consumer/            # Queue-Consumer (Eingang)
    context/             # Context Layer
      graphrag.py        # Vector + Graph + Web Retrieval
      indexer.py         # Codebase-Indexierung
      keywords.py        # KeyBERT Keyword-Extraction
    quality/             # Quality Layer
      debate.py          # Multi-Agent Debate (Pro/Con/Moderator)
      reviewer.py        # Score/Chooser-basierter Solution-Reviewer
      sampler.py         # Action Sampling (AskColleagues, BinaryComparison)
      guardrail.py       # LLM Guardrail Agent (von CrewAI)
      action_node.py     # Structured Output / Schema-Validierung (von MetaGPT)
    routing/             # Routing Layer
      router.py          # Task-basiertes Model-Routing
      cost.py            # Kosten-Tracking und Budgets
    safety/              # Safety Layer
      evaluator.py       # Command Safety Evaluator
      policies.py        # Projekt-spezifische Sicherheitsregeln
      blocklists.py      # Tool-Blocklists (konfigurierbar)
    execution/           # Execution Layer
      sandbox.py         # Docker-Container-Management
      mount.py           # Mount-Modus Logik
      tools.py           # Tool-Provisioning (Shell, File, Git, etc.)
      workbench.py       # Tool-Container mit shared State (von AutoGen)
    memory/              # Memory Layer
      composite.py       # Composite Scoring (Semantic+Recency+Importance)
      context_window.py  # Context-Window-Strategien (Buffered/TokenLimited/HeadAndTail)
      experience.py      # Experience Pool (@exp_cache, von MetaGPT)
    history/             # History Management
      processors.py      # Context-Window-Optimierung (Pipeline)
    hooks/               # Hook-System (Observer-Pattern)
      agent_hooks.py     # Agent-Lifecycle-Hooks
      env_hooks.py       # Environment-Lifecycle-Hooks
    events/              # Event-Bus (von CrewAI)
      bus.py             # Event-Emitter + Subscriber
      types.py           # Agent/Task/System Event-Definitionen
    orchestration/       # Workflow-Orchestrierung
      graph_flow.py      # DAG-Orchestrierung (von AutoGen)
      termination.py     # Composable Termination Conditions
      handoff.py         # HandoffMessage Pattern
      planning.py        # MagenticOne Planning Loop + Stall Detection
      pipeline.py        # Dokument-Pipeline PRD→Design→Tasks→Code (von MetaGPT)
    trajectory/          # Trajectory Recording
      recorder.py        # Schritt-fuer-Schritt Aufzeichnung
      replay.py          # Deterministisches Replay
    agents/              # Agent-Backends (Aider, OpenHands, etc.)
    llm/                 # LLM-Client via LiteLLM
    models/              # Datenmodelle
      components.py      # Component System (JSON-serialisierbare Configs)
    tools/               # YAML-basierte Tool-Bundles
      bundles/           # Tool-Bundle-Verzeichnisse
      recommender.py     # BM25 Tool-Recommendation (von MetaGPT)
    templates/           # Jinja2-Prompt-Templates
    hitl/                # Human-in-the-Loop
      providers.py       # Human Feedback Provider Protocol (von CrewAI)
```

## Framework-Insights: Adoptierte Patterns

Aus der Analyse von LangGraph, CrewAI, AutoGen und MetaGPT wurden folgende
Patterns fuer CodeForge uebernommen. Detaillierter Vergleich: docs/research/market-analysis.md

### Composite Memory Scoring (von CrewAI)

Einfache semantische Aehnlichkeit reicht nicht fuer Memory-Recall. CodeForge
verwendet gewichtetes Scoring aus drei Faktoren:

```
Score = (semantic_weight * cosine_similarity)
      + (recency_weight  * recency_decay)
      + (importance_weight * importance_score)
```

| Faktor | Default-Gewicht | Berechnung |
|---|---|---|
| Semantic | 0.5 | Cosine-Similarity der Embeddings |
| Recency | 0.3 | Exponential Decay (Half-Life konfigurierbar) |
| Importance | 0.2 | LLM-basierte Bewertung bei Speicherung |

Zwei Recall-Modi:
- **Shallow:** Direkter Vector-Search mit Composite Scoring
- **Deep:** LLM destilliert Sub-Queries, sucht parallel, Confidence-basiertes Routing

### Context-Window-Strategien (von AutoGen)

Zusaetzlich zu den History Processors werden verschiedene Strategien fuer
das Chat-Completion-Context-Management unterstuetzt:

| Strategie | Verhalten |
|---|---|
| **Unbounded** | Alle Messages behalten (nur fuer kurze Sessions) |
| **Buffered** | Letzte N Messages behalten |
| **TokenLimited** | Auf Token-Budget trimmen |
| **HeadAndTail** | Erste N + letzte M Messages behalten (System-Prompt + aktueller Kontext) |

Konfigurierbar per Agent-Typ und LLM. Kleine lokale Models bekommen aggressiveres
Trimming, grosse API-Models behalten mehr Kontext.

### Experience Pool (von MetaGPT)

Erfolgreiche Agent-Runs werden gecacht und bei aehnlichen Tasks wiederverwendet:

```
@exp_cache(context_builder=build_task_context)
async def solve_task(task: Task) -> Result:
    # Wenn aehnlicher Task bereits erfolgreich geloest:
    # → Cached Result zurueckgeben
    # Sonst: Normal ausfuehren und Ergebnis cachen
```

- Cache-Key basiert auf Task-Beschreibung + Codebase-Kontext
- Similarity-basiertes Retrieval (nicht exakt-match)
- Konfigurierbare Confidence-Schwelle
- Spart LLM-Kosten und verbessert Konsistenz

### Tool-Recommendation via BM25 (von MetaGPT)

Statt alle verfuegbaren Tools an das LLM zu uebergeben (Token-Verschwendung),
werden relevante Tools automatisch ausgewaehlt:

- BM25-basiertes Ranking von Tools gegen den aktuellen Task-Kontext
- Top-K Tools werden dem LLM als Function Calls angeboten
- Reduziert Token-Usage und verbessert Tool-Auswahl-Qualitaet
- Fallback: Alle Tools bei niedrigem Confidence-Score

### Workbench — Tool-Container (von AutoGen)

Zusammengehoerige Tools teilen sich State und Lifecycle:

```python
class GitWorkbench(Workbench):
    """Git-Tools mit shared Repository-State."""

    def __init__(self, repo_path: str):
        self.repo = git.Repo(repo_path)

    def get_tools(self) -> list[Tool]:
        return [
            Tool("git_status", self._status),
            Tool("git_diff", self._diff),
            Tool("git_commit", self._commit),
            # Tools teilen self.repo
        ]
```

- Shared State zwischen zusammengehoerigen Tools
- Lifecycle-Management (start/stop/restart)
- Dynamische Tool-Discovery (Tools koennen sich aendern)
- Ideal fuer MCP-Integration (McpWorkbench)

### LLM Guardrail Agent (von CrewAI)

Ein dedizierter Agent validiert den Output eines anderen Agents:

```
Agent A (Coder) → Output → Guardrail Agent → Validiert → Accept / Reject + Feedback
                                                              ↓ (bei Reject)
                                                         Agent A wiederholt mit Feedback
```

Integriert in den Quality Layer als vierte Strategie neben
Action Sampling, RetryAgent+Reviewer und Multi-Agent Debate:

| Stufe | Aufwand | Mechanismus |
|---|---|---|
| 1. Action Sampling | Leicht | N Antworten, beste auswaehlen |
| 2. RetryAgent + Reviewer | Mittel | Retry + Score/Chooser Bewertung |
| 3. LLM Guardrail Agent | Mittel | Dedizierter Agent prueft Output |
| 4. Multi-Agent Debate | Schwer | Pro/Con/Moderator |

### Structured Output / ActionNode (von MetaGPT)

LLM-Outputs werden gegen ein Schema validiert und bei Bedarf automatisch korrigiert:

```python
class CodeReviewOutput(ActionNode):
    issues: list[Issue]       # Gefundene Probleme
    severity: str             # critical / warning / info
    suggestion: str           # Verbesserungsvorschlag
    approved: bool            # Review bestanden?
```

- Schema-Definition als Pydantic-Model
- LLM fuellt die Felder via constrained generation
- Automatischer Review/Revise Cycle bei Schema-Verletzung
- Retry mit Fehler-Feedback an das LLM

### Event-Bus fuer Observability (von CrewAI)

Alle relevanten Ereignisse im System werden ueber einen Event-Bus emittiert:

```
Agent Events:          Task Events:           System Events:
  agent_started          task_assigned          budget_warning
  agent_step_done        task_completed         budget_exceeded
  agent_tool_called      task_failed            provider_error
  agent_tool_result      task_retrying          provider_fallback
  agent_thinking         task_guardrail_fail    queue_backpressure
  agent_finished         task_human_input       worker_started
  agent_error            task_delegated         worker_stopped
```

- Events werden ueber WebSocket an das Frontend gestreamt
- Dashboard kann Events filtern, aggregieren, visualisieren
- Monitoring/Alerting auf Event-Basis (z.B. budget_exceeded → Notification)
- Audit-Trail: Alle Events persistiert fuer Nachvollziehbarkeit

### GraphFlow / DAG-Orchestrierung (von AutoGen)

Fuer komplexe Multi-Agent-Workflows mit konditionalen Pfaden:

```
                    ┌─── success ──→ [Test Agent]
[Plan Agent] ──→ [Code Agent] ──┤
                    └─── failure ──→ [Debug Agent] ──→ [Code Agent]
                                                          (Cycle)
```

- Conditional Edges basierend auf Agent-Output
- Parallel Nodes (activation="any" fuer Race, activation="all" fuer Join)
- Cycle-Support mit Exit-Conditions (max_iterations, success_condition)
- DiGraphBuilder API fuer fluent Graph-Konstruktion
- Visualisierbar im Frontend als interaktiver DAG-Editor

### Termination Conditions (von AutoGen)

Flexible, composable Stop-Bedingungen fuer Agent-Workflows:

```python
# Composable mit & (AND) und | (OR)
stop = (MaxSteps(50)
        | BudgetExceeded(max_cost=5.0)
        | TextMention("TASK_COMPLETE")
        | Timeout(minutes=30))
        & NotCondition(StallDetected())
```

Verfuegbare Conditions:
- MaxSteps, MaxMessages, MaxTokens
- BudgetExceeded (Kosten-Limit)
- TextMention (bestimmter Text im Output)
- Timeout (Wanduhr-basiert)
- StallDetected (keine Fortschritte)
- FunctionCallResult (bestimmtes Tool-Ergebnis)
- Custom (beliebige Predicate-Funktion)

### Component System / Deklarative Konfiguration (von AutoGen)

Agents, Tools und Workflows sind als JSON/YAML serialisierbar und
ohne Code-Aenderung rekonstruierbar:

```json
{
  "provider": "codeforge.agents.CodeReviewAgent",
  "version": 1,
  "config": {
    "llm": "claude-sonnet-4-20250514",
    "tools": ["git_diff", "file_read", "lint"],
    "guardrail": "code_quality",
    "max_iterations": 10,
    "budget_limit": 2.0
  }
}
```

- Essentiell fuer den GUI-Workflow-Editor
- Agents/Workflows koennen gespeichert, geteilt und versioniert werden
- Schema-Versionierung mit Migration-Support
- Import/Export von Agent-Konfigurationen

### Dokument-Pipeline PRD→Design→Code (von MetaGPT)

Fuer komplexe Features: Strukturierte Zwischenartefakte statt direkter Code-Generierung:

```
1. Requirement → Strukturiertes PRD (JSON)
     User Stories, Akzeptanzkriterien, Scope
2. PRD → System Design (JSON + Mermaid)
     Datenstrukturen, API-Spezifikation, Klassendiagramm
3. Design → Task-Liste (JSON)
     Geordnete Liste der zu erstellenden Dateien mit Dependencies
4. Tasks → Code (pro Datei)
     Kontext: Design + andere bereits erstellte Dateien
5. Code → Review + Tests
     Automatische Validierung gegen Design-Spezifikation
```

- Jedes Zwischendokument ist schema-validiert (ActionNode)
- Reduziert Halluzination durch strukturierte Constraints
- Incremental Development: Bestehender Code wird beruecksichtigt
- Zwischendokumente sind in der GUI sichtbar und editierbar

### MagenticOne Planning Loop (von AutoGen)

Fuer komplexe, langlebige Tasks: Adaptives Planning mit Stall-Detection:

```
1. PLAN    → Orchestrator erstellt initialen Plan
2. EXECUTE → Agent arbeitet naechsten Schritt ab
3. CHECK   → Fortschritt evaluieren:
               - Fortschritt? → Weiter mit 2
               - Stall?      → Re-Planning (zurueck zu 1)
               - Fertig?     → Ergebnis liefern
               - Gescheitert?→ Fact-Gathering, dann Re-Planning
```

- Stall-Detection erkennt wenn Agents sich im Kreis drehen
- Re-Planning passt den Plan an basierend auf bisherigen Ergebnissen
- Fact-Gathering sammelt fehlende Informationen vor neuem Plan
- Progress-Tracking ueber ein Ledger (Fortschritts-Protokoll)

### HandoffMessage Pattern (von AutoGen)

Agents uebergeben Tasks explizit an Spezialisten:

```
[Planner Agent]
    → HandoffMessage(target="coder", context="Implement feature X per plan")
        → [Code Agent]
            → HandoffMessage(target="reviewer", context="Review changes in src/")
                → [Review Agent]
                    → HandoffMessage(target="tester", context="Run test suite")
                        → [Test Agent]
```

- Explizite Uebergabe mit Kontext (nicht blind weiterleiten)
- Agent entscheidet selbst, an wen uebergeben wird
- Passt zu CodeForge's Agent-Spezialisierung (Planner, Coder, Reviewer, etc.)
- Funktioniert mit verschiedenen Agent-Backends (Aider→OpenHands→SWE-agent)

### Human Feedback Provider Protocol (von CrewAI)

Erweiterbare HITL-Kanaele ueber ein Provider-Interface:

```python
class HumanFeedbackProvider(Protocol):
    async def request_feedback(
        self, context: dict, options: list[str]
    ) -> FeedbackResult:
        ...
```

Implementierungen:
- **WebGuiProvider** — Feedback ueber die SolidJS Web-GUI (Default)
- **SlackProvider** — Approval-Requests als Slack-Messages
- **EmailProvider** — Approval via Email-Link
- **CliProvider** — Terminal-Input fuer Entwicklung/Debugging

## Roadmap/Feature-Map: Auto-Detection & Adaptive Integration

### Grundprinzip

CodeForge erkennt automatisch, welche Spec-Driven-Development-Tools, PM-Plattformen und
Roadmap-Artefakte in einem Projekt verwendet werden, und bietet passende Integration an.
**Kein eigenes PM-Tool** — stattdessen bidirektionaler Sync mit bestehenden Tools.

### Provider Registry fuer Specs und PM

Gleiche Architektur wie `gitprovider` und `llmprovider` — neue Adapter erfordern nur
ein neues Package und einen Blank-Import:

```
port/
  specprovider/
    provider.go        # Interface: Detect(), ReadSpecs(), WriteChange(), Watch()
    registry.go        # Register(), New(), Available()
  pmprovider/
    provider.go        # Interface: Detect(), SyncItems(), CreateItem(), Webhooks()
    registry.go        # Register(), New(), Available()

adapter/
  openspec/            # OpenSpec (openspec/ Verzeichnis)
  speckit/             # GitHub Spec Kit (.specify/ Verzeichnis)
  autospec/            # Autospec (specs/spec.yaml)
  plane/               # Plane.so (REST API v1)
  openproject/         # OpenProject (REST API v3)
  github_pm/           # GitHub Issues/Projects (REST + GraphQL)
  gitlab_pm/           # GitLab Issues/Boards (REST + GraphQL)
```

### Drei-Tier Auto-Detection

```
┌─────────────────────────────────────────────────────────────┐
│                    Auto-Detection Engine                      │
│                                                             │
│  Tier 1: Spec-Driven Detectors (Repo-Dateien)              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ OpenSpec  │ │ Spec Kit │ │ Autospec │ │ ADR/RFC  │      │
│  │openspec/ │ │.specify/ │ │specs/*.y │ │docs/adr/ │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 2: Platform Detectors (API-basiert)                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ GitHub   │ │ GitLab   │ │ Plane.so │ │OpenProj. │      │
│  │Issues/PR │ │Issues/MR │ │REST API  │ │REST API  │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 3: File-Based Detectors (einfache Marker)             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ROADMAP.md│ │TASKS.md  │ │backlog/  │ │CHANGELOG │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
└─────────────────────────────────────────────────────────────┘
```

Jeder Detector implementiert das `specprovider.SpecProvider` oder `pmprovider.PMProvider`
Interface und registriert sich via `init()`. Die Detection-Engine iteriert ueber alle
registrierten Detectors und liefert eine Liste erkannter Tools.

### Spec-Provider Interface

```go
type SpecProvider interface {
    // Detect prueft ob dieses Spec-Format im Repo vorhanden ist
    Detect(repoPath string) (bool, error)

    // ReadSpecs liest alle Specs aus dem Repo
    ReadSpecs(repoPath string) ([]Spec, error)

    // WriteChange schreibt eine Aenderung (Delta-Format)
    WriteChange(repoPath string, change Change) error

    // Watch beobachtet Spec-Aenderungen (fuer bidirektionalen Sync)
    Watch(repoPath string, callback func(event SpecEvent)) error

    // Capabilities deklariert unterstuetzte Operationen
    Capabilities() []Capability
}
```

### PM-Provider Interface

```go
type PMProvider interface {
    // Detect prueft ob diese PM-Plattform fuer das Projekt konfiguriert ist
    Detect(projectConfig ProjectConfig) (bool, error)

    // SyncItems synchronisiert Items bidirektional
    SyncItems(ctx context.Context, direction SyncDirection) (SyncResult, error)

    // CreateItem erstellt ein neues Item auf der Plattform
    CreateItem(ctx context.Context, item Item) (string, error)

    // RegisterWebhook registriert einen Webhook fuer Echtzeit-Sync
    RegisterWebhook(ctx context.Context, callbackURL string) error

    // Capabilities deklariert unterstuetzte Operationen
    Capabilities() []Capability
}
```

### Bidirektionaler Sync

```
┌─────────────────┐              ┌─────────────────┐
│  CodeForge       │  ◄── Sync ──►  │  External PM     │
│  Roadmap-Modell  │              │  (Plane/GitHub/  │
│                  │              │   OpenProject)   │
│  Milestone       │  ←──────→   │  Initiative/     │
│  Feature         │  ←──────→   │  Epic/Issue      │
│  Task            │  ←──────→   │  Work Item       │
└────────┬────────┘              └─────────────────┘
         │
         │  ◄── Sync ──►
         │
┌────────▼────────┐
│  Repo Specs      │
│  (OpenSpec/       │
│   Spec Kit/       │
│   Autospec)       │
└─────────────────┘
```

- **Import:** PM-Tool → CodeForge Roadmap-Modell (Issues/Epics werden zu Features/Tasks)
- **Export:** CodeForge → PM-Tool (neue Features werden als Issues angelegt)
- **Bidirektional:** Aenderungen in beide Richtungen synchronisieren
- **Konflikt-Resolution:** Timestamp-basiert + User-Entscheidung bei Konflikten
- **Sync-Trigger:** Webhook (Echtzeit), Poll (periodisch), Manuell

### Roadmap-Datenmodell

```go
// Internes Roadmap-Modell — PM-Adapter mappen auf dieses Format
type Milestone struct {
    ID          string
    Title       string
    Description string
    DueDate     time.Time
    Features    []Feature
    Status      MilestoneStatus  // planned, active, completed
    LockVersion int              // Optimistic Locking (von OpenProject)
}

type Feature struct {
    ID          string
    Title       string
    Description string
    Priority    Priority
    Tasks       []Task
    Labels      []string         // Label-triggered Sync (von Plane)
    SpecRef     string           // Referenz zu Spec-Datei (openspec/specs/feature.md)
    ExternalIDs map[string]string // {"plane": "abc", "github": "123"}
}
```

### `/ai` Endpoint fuer LLM-Konsum (von Ploi Roadmap)

```
GET /api/v1/projects/{id}/roadmap/ai?format=json
GET /api/v1/projects/{id}/roadmap/ai?format=yaml
GET /api/v1/projects/{id}/roadmap/ai?format=markdown
```

Stellt die Roadmap in einem fuer LLMs optimierten Format bereit:
- Kompakte Zusammenfassung aller Milestones, Features, Tasks
- Status-Informationen und Abhaengigkeiten
- Nutzbar fuer AI-Agents die den Projektkontext verstehen muessen

### Verzeichnisstruktur (Erweiterung)

```
internal/
  port/
    specprovider/          # Spec-Detection Interface
      provider.go          # SpecProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
    pmprovider/            # PM-Platform Interface
      provider.go          # PMProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
  adapter/
    openspec/              # OpenSpec Adapter (openspec/ Verzeichnis)
    speckit/               # GitHub Spec Kit Adapter (.specify/)
    autospec/              # Autospec Adapter (specs/spec.yaml)
    plane/                 # Plane.so REST API v1 Adapter
    openproject/           # OpenProject REST API v3 Adapter
    github_pm/             # GitHub Issues/Projects Adapter
    gitlab_pm/             # GitLab Issues/Boards Adapter
  domain/
    roadmap/               # Roadmap-Domain (Milestone, Feature, Task)
  service/
    detection.go           # Auto-Detection Engine
    sync.go                # Bidirektionaler Sync Service
```

## LLM-Integration: LiteLLM Proxy als Sidecar

### Architekturentscheidung

Nach Analyse von LiteLLM, OpenRouter, Claude Code Router und OpenCode CLI:
**CodeForge baut kein eigenes LLM-Provider-Interface.** LiteLLM Proxy laeuft als Docker-Sidecar
und stellt eine einheitliche OpenAI-kompatible API bereit. Detaillierte Analyse: docs/research/market-analysis.md

### Integrations-Architektur

```
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │  Cost Dashboard  │  Provider Config UI       │   │
│  └──────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ LiteLLM      │  │ Scenario     │                 │
│  │ Config Mgr   │  │ Router       │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ User-Key     │  │ Local Model  │                 │
│  │ Mapping      │  │ Discovery    │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐                                   │
│  │ Copilot      │                                   │
│  │ Token Exch.  │                                   │
│  └──────────────┘                                   │
└────────────┬────────────────────────┬───────────────┘
             │ OpenAI-kompatible API  │
             │ (Port 4000)            │
┌────────────▼────────────────────────┤
│      LiteLLM Proxy (Sidecar)       │
│                                     │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Router      │  │  Budget    │  │
│  │  (6 Strat.)  │  │  Manager   │  │
│  └──────────────┘  └────────────┘  │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Caching     │  │  Callbacks │  │
│  │  (Redis)     │  │(Prometheus)│  │
│  └──────────────┘  └────────────┘  │
└────────────┬────────────────────────┘
             │ Provider APIs
┌────────────▼────────────────────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter  │
└─────────────────────────────────────────────────────┘
```

### Was LiteLLM bereitstellt (nicht selber bauen)

| Feature | LiteLLM-Mechanismus |
|---|---|
| Provider-Abstraktion | 127+ Provider, einheitliche API |
| Routing | 6 Strategien: latency, cost, usage, least-busy, shuffle, tag-based |
| Fallbacks | Fallback-Ketten mit Cooldown (60s default) |
| Cost-Tracking | Per-Call, per-Model, per-Key via Pricing-DB (36.000+ Eintraege) |
| Budgets | Per-Key, Per-Team, Per-User, Per-Provider Limits |
| Streaming | `CustomStreamWrapper` normalisiert alle Provider auf OpenAI SSE |
| Tool-Calling | Einheitlich ueber `tools` Parameter, Provider-Konvertierung automatisch |
| Structured Output | `response_format` cross-provider (nativ oder via Tool-Call-Fallback) |
| Caching | In-Memory, Redis, Semantic (Qdrant), S3, GCS |
| Observability | 42+ Integrations (Prometheus, Langfuse, Datadog, etc.) |
| Rate Limiting | Per-Key TPM/RPM, Per-Team, Per-Model |

### Was CodeForge baut (Eigenentwicklung)

| Komponente | Schicht | Beschreibung |
|---|---|---|
| **LiteLLM Config Manager** | Go Core | Generiert `litellm_config.yaml` aus CodeForge-DB. CRUD fuer Models, Deployments, Keys. |
| **User-Key-Mapping** | Go Core | CodeForge-User → LiteLLM Virtual Keys. API-Keys sicher in CodeForge-DB, Weiterleitung an LiteLLM. |
| **Scenario Router** | Go Core | Task-Typ → LiteLLM-Tag. `metadata.tags: ["think"]` im Request → LiteLLM routet zum passenden Deployment. |
| **Cost Dashboard** | Frontend | LiteLLM Spend API abfragen (`/spend/logs`, `/global/spend/per_team`). Visualisierung pro Projekt/User/Agent. |
| **Local Model Discovery** | Go Core | Ollama (`/api/tags`) und LM Studio (`/v1/models`) Endpoints abfragen. Entdeckte Models automatisch in LiteLLM Config eintragen. |
| **Copilot Token Exchange** | Go Core | GitHub OAuth Token aus `~/.config/github-copilot/hosts.json` lesen, gegen Bearer Token tauschen via `api.github.com/copilot_internal/v2/token`. |

### Scenario-basiertes Routing

Inspiriert von Claude Code Router. Verschiedene Task-Typen werden automatisch an
passende Models geroutet ueber LiteLLM's Tag-based Routing:

```yaml
# litellm_config.yaml (generiert von Go Core)
model_list:
  - model_name: default
    litellm_params:
      model: anthropic/claude-sonnet-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["default", "review"]

  - model_name: background
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: os.environ/OPENAI_API_KEY
      tags: ["background"]

  - model_name: think
    litellm_params:
      model: anthropic/claude-opus-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["think", "plan"]

  - model_name: longcontext
    litellm_params:
      model: google/gemini-2.0-pro
      api_key: os.environ/GEMINI_API_KEY
      tags: ["longContext"]

  - model_name: local
    litellm_params:
      model: ollama/llama3
      api_base: http://ollama:11434
      tags: ["background", "default"]

router_settings:
  routing_strategy: "tag-based-routing"
  num_retries: 3
  fallbacks:
    - default: ["local"]
    - think: ["default"]
```

| Scenario | Wann | Typische Models |
|---|---|---|
| `default` | Allgemeine Coding-Tasks | Claude Sonnet, GPT-4o |
| `background` | Batch, Index, Embedding | GPT-4o-mini, DeepSeek, lokal |
| `think` | Architektur, Debugging, komplexe Logik | Claude Opus, o3 |
| `longContext` | Input > 60K Tokens | Gemini Pro (1M Context) |
| `review` | Code Review, Quality Check | Claude Sonnet |
| `plan` | Feature-Planung, Design-Dokumente | Claude Opus |

### LiteLLM Proxy Konfiguration

```yaml
# docker-compose.yml (Auszug)
services:
  litellm:
    image: docker.litellm.ai/berriai/litellm:main-stable
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: ["--config", "/app/config.yaml", "--port", "4000"]
    environment:
      - LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY}
    depends_on:
      - postgres
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health/liveliness"]
```

### Verzeichnisstruktur Frontend (SolidJS)

```
frontend/
  src/
    features/            # Feature-Module (dashboard, roadmap, agents, llm)
    shared/              # Gemeinsame Komponenten, Primitives, Utils
    api/                 # API-Client, WebSocket-Handler
```
