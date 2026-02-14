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
└────────────┬────────────────────────┬───────────────┘
             │  Message Queue         │
             │  (NATS / Redis)        │
┌────────────▼──────┐  ┌─────────────▼───────────────┐
│  Python Worker 1  │  │  Python Worker N            │
│                   │  │                             │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │  LiteLLM    │  │  │  │  LangGraph  │           │
│  │  (Routing)  │  │  │  │  (Agents)   │           │
│  └─────────────┘  │  │  └─────────────┘           │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │ Agent Exec  │  │  │  │ Agent Exec  │           │
│  │(Aider, etc.)│  │  │  │(OpenHands)  │           │
│  └─────────────┘  │  │  └─────────────┘           │
└───────────────────┘  └─────────────────────────────┘
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
| Python Workers → LLM APIs | HTTPS | LLM-Calls via LiteLLM |
| Go → SCM (Git/SVN) | CLI / REST API | Repo-Operationen |

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

Standardisierter Workflow fuer alle nicht-autonomen Agents:

```
1. PLAN      Agent analysiert Task + Codebase, erstellt strukturierten Plan
                ↓
2. APPROVE   Plan wird dem User zur Freigabe vorgelegt (Web-GUI)
                ↓  (User kann ablehnen, aendern, oder auto-approve setzen)
3. EXECUTE   Agent arbeitet Plan Punkt fuer Punkt ab
                ↓
4. REVIEW    Automatisches Self-Review oder zweiter Agent prueft Ergebnis
                ↓
5. DELIVER   Ergebnis als Diff/Patch, PR, oder direkte Dateiaenderung
```

- Vollwertige Agents (Claude Code) koennen autonom arbeiten — Workflow optional
- API-basierte LLMs brauchen den strukturierten Workflow
- Jeder Schritt ist einzeln konfigurierbar (Skip, Auto-Approve, etc.)
- Human-in-the-Loop an jedem Schritt moeglich ueber die Web-GUI

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

### Agent-Spezialisierung (offener Punkt)

Statt eines General-Purpose-Agents koennen spezialisierte Sub-Agents
(Planner, Coder, Reviewer, Researcher, etc.) ueber YAML-Configs definiert
und in der GUI als eigene Workflows konfiguriert werden.

**Dieser Punkt wird zu einem spaeteren Zeitpunkt detailliert ausgearbeitet.**

Geplant:
- Default-YAML-Configs fuer gaengige Spezialisierungen
- GUI-Workflow-Editor zur Konfiguration der Agent-Pipeline
- Konfigurierbare Reihenfolge und Aktivierung von Sub-Agents

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

### Verzeichnisstruktur Frontend (SolidJS)

```
frontend/
  src/
    features/            # Feature-Module (dashboard, roadmap, agents, llm)
    shared/              # Gemeinsame Komponenten, Primitives, Utils
    api/                 # API-Client, WebSocket-Handler
```
