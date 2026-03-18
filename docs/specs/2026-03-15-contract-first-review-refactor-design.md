# Design Spec: Contract-First Review/Refactor System (Phase 31)

## Context

CodeForge soll als Produkt-Feature automatische Code-Reviews und Refactoring-Zyklen fuer die Projekte anbieten, die es orchestriert. Inspiriert durch eine Perplexity-Analyse (validiert gegen den echten Codebase-Stand) und kollaboratives Brainstorming.

**Problem:** Projekte akkumulieren technische Schulden zwischen Features. Cross-Layer-Contracts driften auseinander. Refactoring passiert ad-hoc oder gar nicht. Es fehlt ein systematischer, in den Agent-Workflow integrierter Review/Refactor-Zyklus.

**Loesung:** Ein Contract-First Review/Refactor-System das:
- Boundary-Files (Schnittstellen zwischen Modulen/Layern/Sprachen) automatisch erkennt (LLM-basiert)
- Zweistufig reviewt: erst Cross-Layer-Contracts, dann Intra-Layer-Code-Qualitaet
- Refactoring-Vorschlaege generiert und bei grossen Aenderungen auf User-Approval wartet
- Durch eine Kaskade aus drei Triggern ausgeloest wird (Pipeline-Completion, Branch-Merge, manuell)
- Phase-aware LiteLLM-Routing nutzt fuer Token-Effizienz

**Architektur-Ansatz:** Hybrid Pipeline-Native + Agent-Driven
- Go Core steuert den Fluss deterministisch (wann, ob, in welcher Reihenfolge)
- Python/LLM liefert die Inhalte (was sind Boundaries, was muss refactored werden)
- Baut auf existierender Pipeline/Orchestrator/Modes-Infrastruktur auf

---

## 1. Gesamtarchitektur

```
Trigger-Kaskade (Go Core)
  +-- Pipeline-Completion Event
  +-- Branch-Merge Detection
  +-- Manueller API/Chat-Befehl
        |
        v
ReviewTriggerService (Go Core)
  -> Deduplizierung (gleicher Commit-SHA -> skip)
  -> Prueft ob Review fuer Projekt konfiguriert
        |
        v
Contract-First Review Pipeline (Orchestrator)
  Step 0: boundary-analyzer    (LLM identifiziert/prueft Boundaries)
  Step 1: contract-reviewer    (LLM prueft Cross-Layer-Contracts)
  Step 2: intra-layer-reviewer (LLM reviewt Layer-intern)
  Step 3: refactorer           (LLM schlaegt Refactorings vor)
        |
        v
Threshold-HITL-Gate (Policy Engine)
  Diff-Groesse < Schwellwert? -> auto-apply
  Diff-Groesse >= Schwellwert? -> DecisionAsk -> WebSocket -> User-Approve
        |
        v
Quality Gate (Test/Lint) -> Commit
```

### Neue Komponenten

| Komponente | Layer | Beschreibung |
|---|---|---|
| `ProjectBoundaryConfig` | Go Domain | Gespeicherte Boundary-Files pro Projekt |
| `boundary-analyzer` Mode | Go Preset + Python Agent | LLM-basierte Boundary-Erkennung |
| `contract-reviewer` Mode | Go Preset + Python Agent | Cross-Layer-Contract-Pruefung |
| `review-refactor` Pipeline | Go Pipeline-Template | 4-Step sequentielle Pipeline |
| `ReviewTriggerService` | Go Service | Kaskaden-Trigger + Deduplizierung |
| `DiffImpactScorer` | Go Service | Threshold-basierte HITL-Entscheidung |
| `waiting_approval` Status | Go Domain (Run/Step) | Neuer Workflow-Status fuer HITL-Pause |
| HITL WebSocket Events | Go + Frontend | `refactor.approval_required` Event + Approval-UI |

### Existierende Infrastruktur (wird wiederverwendet)

- `OrchestratorService` -- DAG, State Machine, SharedContext (`internal/service/orchestrator.go`)
- `reviewer` Mode -- Steps 2 nutzt existierenden Mode (`internal/domain/mode/presets.go`)
- `refactorer` Mode -- Step 3 nutzt existierenden Mode (`internal/domain/mode/presets.go`)
- Policy Engine -- `DecisionAsk` Pattern (`internal/domain/policy/policy.go`)
- LiteLLM Hybrid Router -- Scenario-Tags `plan`, `review` (`workers/codeforge/routing/`)
- Quality Gate -- Test/Lint nach Refactoring (`workers/codeforge/qualitygate.py`)
- Git Provider -- Diff-Analyse (`internal/adapter/gitlocal/provider.go`)
- Shadow Git Checkpoints -- Rollback (`internal/service/checkpoint.go`)
- NATS JetStream -- Alle Kommunikation Go <-> Python
- AG-UI WebSocket Events -- Frontend-Updates

---

## 2. Boundary-Detection und Onboarding

### Ablauf

1. `autoIndexProject()` in `internal/adapter/http/handlers.go` triggert bereits RepoMap + Retrieval-Index
2. Danach startet ein `boundary-analyzer` Run (LLM-basiert)
3. Das LLM analysiert die Projektstruktur und identifiziert:
   - API-Boundaries: OpenAPI/Swagger specs, Protobuf, GraphQL schemas
   - Data-Layer-Boundaries: ORM models, DB migrations, shared types
   - Inter-Service-Boundaries: Message schemas, event definitions, RPC interfaces
   - Cross-Language-Boundaries: Shared JSON contracts zwischen Sprachen
4. Ergebnis wird als `ProjectBoundaryConfig` in PostgreSQL gespeichert
5. User kann im Frontend die erkannten Boundaries einsehen und anpassen

### Domain Model

```go
// internal/domain/boundary/boundary.go
type ProjectBoundaryConfig struct {
    ProjectID    string
    Boundaries   []BoundaryFile
    LastAnalyzed time.Time
    Version      int // Optimistic Locking
}

type BoundaryFile struct {
    Path         string // e.g. "api/schemas/user.proto"
    Type         string // "api" | "data" | "inter-service" | "cross-language"
    Counterpart  string // optional counterpart, e.g. "frontend/src/types/user.ts"
    AutoDetected bool   // LLM-detected vs. manually added
}
```

### DB Migration

- New table `project_boundaries` (project_id, boundaries JSONB, last_analyzed, version)
- Tenant-scoped: `tenant_id` + foreign key on `projects`

### Mode: `boundary-analyzer`

- Tools: Read, Glob, Grep, ListDir (read-only)
- LLMScenario: `plan` (needs good reasoning)
- RequiredArtifact: `BOUNDARIES.json` (structured output)
- Autonomy: 4 (full-auto)
- Re-analysis: Delta-update each review cycle instead of full scan

### API Endpoints

- `GET /api/v1/projects/{id}/boundaries` -- read current boundary config
- `PUT /api/v1/projects/{id}/boundaries` -- manually adjust boundaries
- `POST /api/v1/projects/{id}/boundaries/analyze` -- trigger re-analysis

---

## 3. Contract-First Review Pipeline

### Pipeline-Template: `review-refactor`

```go
// internal/domain/pipeline/presets.go
{
    ID:       "review-refactor",
    Name:     "Contract-First Review & Refactor",
    Protocol: sequential,
    Steps: []Step{
        {Name: "Boundary Analysis",       ModeID: "boundary-analyzer",  DependsOn: nil},
        {Name: "Contract Review",         ModeID: "contract-reviewer",  DependsOn: []int{0}},
        {Name: "Intra-Layer Review",      ModeID: "reviewer",           DependsOn: []int{1}},
        {Name: "Refactoring Proposals",   ModeID: "refactorer",         DependsOn: []int{2}},
    },
}
```

### Steps

- Step 0 `boundary-analyzer`: Delta-update ProjectBoundaryConfig. If no boundaries found, pipeline continues (standard review).
- Step 1 `contract-reviewer` (NEW): Read-only tools, checks contract consistency across layers. Output: `CONTRACT_REVIEW.md`.
- Step 2 `reviewer` (existing): Reuse existing mode. Additional context from Step 1. Output: `REVIEW.md`.
- Step 3 `refactorer` (existing): Gets CONTRACT_REVIEW.md + REVIEW.md as context. Output: DIFF. Threshold-HITL applies here.

### Context Passing

Orchestrator uses SharedContext between pipeline steps. Review outputs stored as artifacts and injected into next step (existing pattern in `orchestrator.go`).

---

## 4. Threshold-based HITL

### Three Tiers

| Impact | Threshold (configurable) | Action |
|---|---|---|
| Low | < `auto_apply_threshold` (default: 50 lines, single-layer) | Auto-apply |
| Medium | >= `auto_apply_threshold` and < `approval_threshold` | Auto-apply + Notification |
| High | >= `approval_threshold` (default: 200 lines, or cross-layer, or structural) | HITL-Pause |

### On HITL-Pause

1. Run status -> `waiting_approval` (new status)
2. WebSocket event `refactor.approval_required` to frontend
3. Frontend shows Diff-Preview with Approve/Reject/Modify buttons
4. User can: Approve (apply all), Partial-Approve, Reject, Discuss (open chat)
5. After decision: Pipeline resumes -> Quality Gate -> Commit

### New Run Status: `waiting_approval`

Extends `internal/domain/plan/plan.go`. Orchestrator pauses `advancePlan()` when a step is `waiting_approval`.

### API Endpoints

- `POST /api/v1/runs/{id}/approve`
- `POST /api/v1/runs/{id}/reject`
- `POST /api/v1/runs/{id}/approve-partial`

---

## 5. Cascade Trigger System

### ReviewTriggerService

Central service that deduplicates all trigger sources and starts the pipeline.

- Trigger A: Pipeline-Completion (auto) -- after standard-dev pipeline completes
- Trigger B: Branch-Merge (auto) -- webhook or polling for merge events
- Trigger C: Manual -- API endpoint or chat command
- Dedup: Same commit SHA within 30min -> skip (except manual)

### NATS Subjects

```
review.trigger.request
review.trigger.complete
review.boundary.analyzed
review.approval.required
review.approval.response
```

---

## 6. Phase-aware LiteLLM-Routing

| Pipeline-Step | LLMScenario Tag | Model Class |
|---|---|---|
| boundary-analyzer | `plan` | Strong (Sonnet/Opus) |
| contract-reviewer | `review` | Medium (GPT-4o-mini/Haiku) |
| reviewer | `review` | Medium |
| refactorer | `plan` | Strong |
| Quality Gate | -- | No LLM |

Adaptive Context Budget per phase via `AdaptiveContextBudget()` extension.
