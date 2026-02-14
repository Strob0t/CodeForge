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
