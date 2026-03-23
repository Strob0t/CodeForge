# Knowledge System Integration — Design Spec

**Date:** 2026-03-23
**Type:** Connect docs-mcp-server + MCP project assignment UI + knowledge pipeline fixes
**Research basis:** 4 Recherchen (MCP UI patterns, CodeForge UI analysis, 25+ academic papers, docs-mcp-server API)

---

## 1. Problem

CodeForge hat ein Knowledge-System das nicht end-to-end funktioniert:
- docs-mcp-server läuft in Docker, aber der Agent kann es nicht nutzen (kein Projekt-Assignment im UI)
- Knowledge Base → Context Pipeline Code existiert, aber KB-Erstellung ist umständlich (nur Dateipfade)
- Stack Detection erkennt Frameworks, nutzt das Wissen aber nicht

## 2. Akademische Grundlage

| Paper | Kernaussage | Relevanz |
|---|---|---|
| CodeRAG-Bench (NAACL 2025) | API-Docs bringen +83-220% für seltene Libraries | docs-mcp-server search_docs direkt nutzbar |
| AllianceCoder (2025) | Ähnlicher Code als Context SCHADET (-15%) | Retrieval soll API-Docs priorisieren, nicht "similar code" |
| "When LLMs Meet API Docs" (2025) | Code-Beispiele in Docs sind der wertvollste Context | docs-mcp-server indexiert inkl. Beispiele |
| Context Length Hurts (2025) | Performance sinkt 14-85% bei längerem Context | Kurze, gezielte Docs statt ganzer Manuals |
| Small Model RAG (2026) | Modelle <7B nutzen RAG-Context nicht (85-100% Failure) | Capability-Level bestimmt ob Docs injiziert werden |
| RAG Equalizer (Pinecone 2024) | Mit guter Retrieval kommt Mixtral auf 3% an GPT-4 | docs-mcp-server kann die Qualitätslücke schließen |

## 3. Lösung: 3 Maßnahmen

### M1: MCP Server → Project Assignment UI

**Was:** Button in den Project Settings um MCP Server dem Projekt zuzuweisen.

**UI-Pattern (nach Recherche):**
- Roo Code Pattern: "Edit Project MCP" Button in Settings
- VS Code Pattern: Enable/Disable separat von Assignment (kein VCS-Conflict)
- Cursor Pattern: Grüner/Roter Status-Dot pro Server

**Implementierung:**
- `CompactSettingsPopover.tsx` erweitern um MCP-Sektion
- Checkbox-Liste aller globalen MCP Server
- Toggle zum Zuweisen/Entfernen
- Status-Indikator (connected/disconnected)
- API existiert bereits: `listProjectServers()`, `assignToProject()`, `unassignFromProject()`
- Frontend API Client existiert bereits — wird nur nie aufgerufen

### M2: docs-mcp-server Docker-Config vervollständigen

**Was:** Ollama-Embeddings konfigurieren (kein OpenAI-Key nötig), Web-UI Port exponieren, Health-Check fixen.

**Änderungen an docker-compose.yml:**
```yaml
docs-mcp:
  environment:
    OPENAI_API_KEY: "ollama"
    OPENAI_API_BASE: "http://host.docker.internal:11434/v1"
    DOCS_MCP_EMBEDDING_MODEL: "nomic-embed-text"
  ports:
    - "6280:6280"   # MCP Server (SSE)
    - "6281:6281"   # Web Dashboard
```

### M3: docs/dev-setup.md Dokumentation

**Was:** Anleitung wie User docs-mcp-server nutzen:
1. `docker compose up docs-mcp` starten
2. Web-UI öffnen (localhost:6281) und Docs scrapen
3. In CodeForge Project Settings: docs-mcp-server dem Projekt zuweisen
4. Agent hat automatisch `search_docs`, `scrape_docs` etc. als MCP-Tools

---

## 4. Files

| File | Action | Purpose |
|---|---|---|
| `frontend/src/features/project/CompactSettingsPopover.tsx` | Modify | MCP Server Assignment UI |
| `docker-compose.yml` | Modify | Ollama embeddings + Web-UI Port |
| `docs/dev-setup.md` | Modify | docs-mcp-server Nutzungsanleitung |
| `docs/features/01-project-dashboard.md` | Modify | MCP-per-Project Feature dokumentieren |

## 5. Was wir NICHT machen

- Kein Hardcoding von Framework-Wissen
- Keine eigene Scraping-Engine (docs-mcp-server macht das)
- Keine KB ↔ MCP Verknüpfung (parallele Systeme, YAGNI)
- Kein Scope-based MCP Resolution (Project-Level reicht)
- Kein Auto-Provisioning (User managed docs-mcp-server selbst)
