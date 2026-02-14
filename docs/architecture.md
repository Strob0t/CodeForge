# CodeForge — Architektur

## Ueberblick

CodeForge ist ein containerisierter Service zur Orchestrierung von AI-Coding-Agents.
Die Architektur folgt einem Drei-Schichten-Modell mit strikter Sprachtrennung nach Aufgabe.

## Systemarchitektur

```
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│              (React / Next.js / Svelte)              │
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

**Quality Layer — Multi-Agent Debate**
- Pro-Agent argumentiert fuer eine Loesung
- Con-Agent sucht Schwachstellen
- Moderator synthetisiert das Ergebnis
- Optional und konfigurierbar — bei einfachen Tasks unnoetig, bei kritischen Changes wertvoll
- Funktioniert mit jedem LLM, auch mit guenstigen/lokalen Models

**Routing Layer — Intelligentes Model-Routing**
- Task-Klassifikation: Architektur, Code-Generierung, Review, Docs, Tests
- Kosten-Optimierung: Einfache Tasks an guenstige Models, komplexe an starke
- Latenz-Routing: Schnelle Antworten fuer interaktive Nutzung
- Fallback-Ketten: Wenn ein Provider ausfaellt, automatisch naechsten nutzen
- Routing-Regeln konfigurierbar per Projekt und per User

### Verzeichnisstruktur Python Workers

```
workers/
  codeforge/
    consumer/            # Queue-Consumer (Eingang)
    context/             # Context Layer
      graphrag.py        # Vector + Graph + Web Retrieval
      indexer.py         # Codebase-Indexierung
    quality/             # Quality Layer
      debate.py          # Multi-Agent Debate (Pro/Con/Moderator)
    routing/             # Routing Layer
      router.py          # Task-basiertes Model-Routing
      cost.py            # Kosten-Tracking und Budgets
    agents/              # Agent-Backends (Aider, OpenHands, etc.)
    llm/                 # LLM-Client via LiteLLM
    models/              # Datenmodelle
```

### Verzeichnisstruktur Frontend

```
frontend/
  src/
    features/            # Feature-Module (dashboard, roadmap, agents, llm)
    shared/              # Gemeinsame Komponenten, Hooks, Utils
    api/                 # API-Client, WebSocket-Handler
```
