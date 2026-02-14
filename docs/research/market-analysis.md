# CodeForge — Marktanalyse & Recherche

> Stand: 2026-02-14

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
- **Beschreibung:** Graph-basierte Agent-Orchestrierung von LangChain. Conditional Logic, Multi-Team, Hierarchische Kontrolle.
- **Relevanz:** Potentielles Backend-Framework fuer Agent-Orchestrierung
- **Hinweis:** Schnellstes Framework mit niedrigster Latenz

### CrewAI
- **URL:** https://www.crewai.com/open-source
- **Beschreibung:** Role-based Task Delegation. CrewAI Studio fuer visuelles Bauen. Hunderte Built-in Tools.
- **Relevanz:** Production-ready Multi-Agent-Framework

### AutoGen (Microsoft)
- **URL:** https://github.com/microsoft/autogen
- **Beschreibung:** Multi-Agent-Systems mit Custom Tools, Memory, Human-in-the-loop.
- **Relevanz:** Gut fuer Prototyping und flexible Agent-Kommunikation

### MetaGPT
- **URL:** https://github.com/geekan/MetaGPT
- **Beschreibung:** Simuliert Development-Teams (Product Manager, Architect, Engineer). Single-Prompt zu komplexen Software-Artefakten.
- **Relevanz:** Interessantes Konzept fuer Team-Simulation

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
