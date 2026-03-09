# Goal System Redesign

**Date:** 2026-03-09
**Status:** Approved
**Scope:** Goal discovery workflow, propose_goal tool, AG-UI integration, GSD-style prompts

## Problem Statement

The current goal system is broken in three ways:

1. **401 Unauthorized** — The `manage_goals` tool makes HTTP callbacks from the Python worker to Go Core using `CODEFORGE_INTERNAL_KEY`. This key is not wired into docker-compose, causing all goal API calls to fail with 401.
2. **Agent skips interview** — The `goal-researcher` mode prompt is too weak. The agent ignores the "explore first, ask questions, then create goals" methodology and immediately generates goals.
3. **Garbled fallback** — After tool errors, the routing system falls back to `groq/llama-3.1-8b-instant` (6000 TPM), which produces incoherent output for this task.

## Design Decisions

### D1: Goals are created in conversation, not via side-effect API calls

The agent and user collaborate in a conversation to discover and refine goals. The agent **proposes** goals; it never creates them directly. The user has approval power over every goal.

### D2: `propose_goal` tool emits AG-UI events instead of HTTP callbacks

The new `propose_goal` tool does not call the Go Core HTTP API. Instead, it emits a `goal_proposal` AG-UI event over the existing WebSocket stream. The frontend renders the proposal with Approve/Edit/Reject buttons.

- **Approve** — Frontend persists the goal via `POST /api/v1/projects/{id}/goals` using the user's JWT. Sends `[Goal approved: {title}]` as the next chat message. Agent then writes the corresponding `docs/` file.
- **Edit** — User types feedback in the chat. Next agent turn refines the proposal.
- **Reject** — Sends `[Goal rejected: {title}]` as the next chat message. Agent moves on.

This eliminates the `CODEFORGE_INTERNAL_KEY` dependency for goals entirely.

### D3: Hybrid persistence — `docs/` files as source of truth, DB as UI index

Goals are persisted in two places:

| Location | Role | Written by |
|---|---|---|
| `docs/PROJECT.md` | Source of truth — vision, constraints, key decisions | Agent (Write tool) |
| `docs/REQUIREMENTS.md` | Source of truth — functional requirements with IDs | Agent (Write tool) |
| `docs/STATE.md` | Source of truth — current state, blockers | Agent (Write tool) |
| PostgreSQL `goals` table | UI index for GoalsPanel display | Frontend (REST API) |

File format follows the GSD framework templates (see Appendix A).

### D4: GSD-style questioning methodology

The `goal-researcher` mode prompt is completely rewritten using GSD's (Get-Shit-Done) questioning methodology:

- **Dream extraction, not requirements gathering** — collaborative thinking, not interrogation
- **Follow energy** — dig into what excites the user, not a checklist
- **Challenge vagueness** — "good" and "simple" need concrete definitions
- **Freeform rule** — when the user wants to explain freely, stop structured questions
- **Context checklist** — track mentally: What, Why, Who, Done-criteria
- **Decision gate** — explicit "Ready to create goals?" before proposing anything
- **Anti-patterns** — no checklist-walking, no canned questions, no rushing

Source: [gsd-build/get-shit-done](https://github.com/gsd-build/get-shit-done) questioning.md

### D5: Context injection instead of read-tool for existing goals

When a goal-discovery conversation starts, Go Core reads `docs/PROJECT.md`, `docs/REQUIREMENTS.md`, and `docs/STATE.md` from the project workspace and injects them as `context_entries` in the NATS payload. The agent has existing goals in context immediately — no HTTP read-tool needed.

### D6: Prompt-based phase enforcement (no tool gating)

The three phases (Explore → Interview → Create) are enforced via the prompt, not by dynamically enabling/disabling tools. Rationale:

- Modes are static per conversation — tool sets cannot change mid-conversation
- The policy layer is for security, not workflow orchestration
- The Approve/Reject buttons are a natural safety net — if the agent proposes too early, the user rejects
- GSD itself uses pure prompt-based enforcement successfully

### D7: Write permission scoped to `docs/`

The `goal-researcher` mode gets Write permission but only needs it for three files:

- `Tools: ["Read", "Glob", "Grep", "ListDir", "propose_goal", "Write"]`
- `DeniedTools: ["Edit", "Bash"]`
- The prompt constrains Write usage to `docs/PROJECT.md`, `docs/REQUIREMENTS.md`, `docs/STATE.md`

## Architecture

### Data Flow

```
User clicks "AI Discover" in GoalsPanel
        |
        v
Go Core: creates Conversation with mode "goal-researcher"
Go Core: reads docs/PROJECT.md, REQUIREMENTS.md, STATE.md from workspace
Go Core: injects file contents as context_entries in NATS payload
        |
        v  NATS (conversation.run.start)
Python Worker: Agent-Loop with GSD-style prompt
        |
        |-- Phase 1: Explore codebase (Read, Glob, Grep)
        |-- Phase 2: Interview user (GSD questioning methodology)
        |-- Phase 3: Create goals (propose_goal tool)
                |
                v  AG-UI Event "goal_proposal" via WebSocket
            Frontend: renders Goal-Card inline in chat
                |
                |-- Approve --> Frontend: POST /api/v1/projects/{id}/goals (User JWT)
                |               Chat message "[Goal approved]" --> Agent writes docs/*.md
                |-- Edit -----> User types feedback --> next Agent turn
                |-- Reject ---> Chat message "[Goal rejected]" --> Agent continues
```

### Components Changed

| Component | Change |
|---|---|
| `workers/codeforge/tools/manage_goals.py` | **Delete entirely** |
| `workers/codeforge/tools/propose_goal.py` | **New** — emits AG-UI event, no HTTP |
| `workers/codeforge/consumer/_conversation.py` | Replace `_register_goals_tool` to use `propose_goal` |
| `internal/domain/mode/presets.go` | Rewrite `goal-researcher` mode (tools + prompt) |
| `internal/adapter/http/handlers_goals.go` | Add context injection in `AIDiscoverProjectGoals` |
| `internal/port/messagequeue/schemas.go` | No change needed (context_entries already exists) |
| AG-UI event types (Go + Python + TypeScript) | **New** `goal_proposal` event type |
| `frontend/src/features/project/GoalsPanel.tsx` | No change (existing CRUD UI stays) |
| Frontend chat components | **New** GoalProposalCard component with Approve/Edit/Reject |

### What Gets Removed

- `manage_goals` tool (Python) — entire file deleted
- HTTP callbacks from agent to Go Core for goals — eliminated
- `CODEFORGE_INTERNAL_KEY` dependency for goal operations — eliminated
- Direct goal creation without user approval — eliminated

## propose_goal Tool Specification

### Tool Definition

```python
PROPOSE_GOAL_DEFINITION = ToolDefinition(
    name="propose_goal",
    description="Propose a project goal for user review. The goal is NOT created "
                "until the user approves it in the UI.",
    parameters={
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["create", "update", "delete"],
                "description": "The proposal action.",
            },
            "kind": {
                "type": "string",
                "enum": ["vision", "requirement", "constraint", "state", "context"],
                "description": "Goal category.",
            },
            "title": {"type": "string", "description": "Goal title."},
            "content": {"type": "string", "description": "Goal content (markdown)."},
            "priority": {
                "type": "integer",
                "description": "Priority 0-100 (default 90).",
            },
            "goal_id": {
                "type": "string",
                "description": "Existing goal ID (required for update/delete).",
            },
        },
        "required": ["action", "kind", "title", "content"],
    },
)
```

### Executor Behavior

1. Validate parameters locally (no HTTP call)
2. Generate a `proposal_id` (UUID)
3. Emit AG-UI event `goal_proposal` via the agent loop's event emitter
4. Return `ToolResult(output="Goal proposed for review: {title}")`

### AG-UI Event Format

```json
{
    "type": "goal_proposal",
    "data": {
        "proposal_id": "uuid",
        "action": "create",
        "kind": "requirement",
        "title": "Product search across multiple sources",
        "content": "A search function that aggregates products from...",
        "priority": 90,
        "goal_id": null
    }
}
```

## Goal-Researcher Mode Prompt

### PromptPrefix Structure

```
# Goal Researcher Mode

You are a thinking partner helping the user discover and articulate project
goals. This is dream extraction, not requirements gathering.

## Tools
- Read, Glob, Grep, ListDir: explore the codebase
- propose_goal: propose a goal for user review (Approve/Edit/Reject)
- Write: persist approved goals to docs/PROJECT.md, docs/REQUIREMENTS.md, docs/STATE.md

## Phase 1: Explore (no user input needed)
Use Glob and Read to understand the project:
- README.md, CLAUDE.md, docs/, package.json/go.mod/pyproject.toml
- If existing goals appear in your context, acknowledge them
- Present a brief summary of what you found
- Do NOT ask questions yet — just present findings

## Phase 2: Deep Questioning
Follow the questioning guide below. Your job:
- Help the user sharpen a fuzzy idea into concrete goals
- Follow threads — dig into what excites them
- Challenge vagueness — "good" means what? "users" means who?
- Track mentally: What are they building? Why? For whom? What does done look like?

Decision gate: When you have enough clarity, ask:
"I think I understand what you're after. Ready to start creating goals?"
If "Keep exploring" — ask what they want to add, or probe gaps.

## Phase 3: Create Goals (incremental with approval)
- One goal at a time via propose_goal
- After each user approval, write the corresponding docs/ file:
  - Vision, Constraints, Key Decisions -> docs/PROJECT.md
  - Requirements -> docs/REQUIREMENTS.md
  - Current State, Blockers -> docs/STATE.md
- Use the file templates below
- After all goals: summarize and ask if adjustments needed

## File Templates

### docs/PROJECT.md
[GSD PROJECT.md template — see Appendix A]

### docs/REQUIREMENTS.md
[GSD REQUIREMENTS.md template — see Appendix A]

### docs/STATE.md
[GSD STATE.md template — see Appendix A]

<questioning_guide>

Project initialization is dream extraction, not requirements gathering.
You are helping the user discover and articulate what they want to build.
This is not a contract negotiation — it is collaborative thinking.

<philosophy>
You are a thinking partner, not an interviewer.
The user often has a fuzzy idea. Your job is to help them sharpen it.
Ask questions that make them think "oh, I hadn't considered that" or
"yes, that's exactly what I mean."
Do not interrogate. Collaborate. Do not follow a script. Follow the thread.
</philosophy>

<the_goal>
By the end of questioning, you need enough clarity to create goals that
downstream phases can act on:
- Research needs: what domain to research, what unknowns exist
- Requirements: clear enough vision to scope v1 features
- Roadmap: clear enough vision to decompose into phases
- Execution: success criteria to verify against, the "why" behind requirements
A vague set of goals forces every downstream phase to guess. The cost compounds.
</the_goal>

<how_to_question>
Start open. Let them dump their mental model. Do not interrupt with structure.
Follow energy. Whatever they emphasized, dig into that.
Challenge vagueness. Never accept fuzzy answers.
Make the abstract concrete. "Walk me through using this."
Clarify ambiguity. "When you say Z, do you mean A or B?"
Know when to stop. When you understand what, why, who, and done — offer to proceed.
</how_to_question>

<question_types>
Use as inspiration, not a checklist. Pick what is relevant to the thread.

Motivation — why this exists:
- "What prompted this?"
- "What are you doing today that this replaces?"

Concreteness — what it actually is:
- "Walk me through using this"
- "Give me an example"

Clarification — what they mean:
- "When you say Z, do you mean A or B?"
- "Tell me more about that"

Success — how you will know it is working:
- "How will you know this is working?"
- "What does done look like?"
</question_types>

<freeform_rule>
When the user wants to explain freely, STOP using structured questions.
If the user signals they want to describe something in their own words,
ask follow-ups as plain text and wait for natural responses.
Resume structured questions only after processing their freeform response.
</freeform_rule>

<context_checklist>
Track mentally — not as conversation structure:
- [ ] What they are building (concrete enough to explain to a stranger)
- [ ] Why it needs to exist (the problem or desire driving it)
- [ ] Who it is for (even if just themselves)
- [ ] What "done" looks like (observable outcomes)
</context_checklist>

<decision_gate>
When you could create clear goals, offer to proceed:
"I think I understand what you're after. Ready to start creating goals?"
Options: "Create goals" or "Keep exploring — I want to share more"
If "Keep exploring" — ask what they want to add or identify gaps and probe.
Loop until ready.
</decision_gate>

<anti_patterns>
- Checklist walking — going through categories regardless of what they said
- Canned questions — "What is your core value?" regardless of context
- Corporate speak — "What are your success criteria?" "Who are your stakeholders?"
- Interrogation — firing questions without building on answers
- Rushing — minimizing questions to get to "the work"
- Shallow acceptance — taking vague answers without probing
- Premature constraints — asking about tech stack before understanding the idea
- Generating ALL goals at once instead of one-by-one with approval
- Creating goals before completing the interview phase
</anti_patterns>

</questioning_guide>
```

## Appendix A: File Templates

### docs/PROJECT.md

```markdown
# [Project Name]

## What This Is
[2-3 sentences. What does this product do and who is it for?]

## Core Value
[The ONE thing that matters most. One sentence.]

## Constraints
- **[Type]**: [What] -- [Why]

## Key Decisions
| Decision | Rationale | Outcome |
|----------|-----------|---------|
| [Choice] | [Why]     | Pending |

---
*Last updated: [date] after [trigger]*
```

### docs/REQUIREMENTS.md

```markdown
# Requirements

**Core Value:** [from PROJECT.md]

## v1
- [ ] [CATEGORY]-[NUMBER]: [User-centric, testable requirement]

## v2
- [CATEGORY]-[NUMBER]: [Deferred requirement]

## Out of Scope
- [Exclusion] -- [why]

## Traceability
| Requirement | Phase | Status |
|-------------|-------|--------|
| [ID]        | -     | Pending |
```

### docs/STATE.md

```markdown
# Project State

## Current Position
Phase: 0 of ? (Goal Discovery)
Status: [status]
Last activity: [date] -- [what happened]

## Decisions
(Logged in PROJECT.md Key Decisions table)

## Blockers
None yet.
```

## Migration Path

1. Delete `workers/codeforge/tools/manage_goals.py`
2. Create `workers/codeforge/tools/propose_goal.py`
3. Add `goal_proposal` AG-UI event type to Go, Python, and TypeScript
4. Create frontend `GoalProposalCard` component
5. Rewrite `goal-researcher` mode in `presets.go`
6. Add context injection for `docs/*.md` files in conversation start
7. Update `_register_goals_tool` in `_conversation.py`
8. Update tests

## Out of Scope

- GSD Steps 5-9 (Workflow Preferences, Research agents, Roadmap creation) — separate feature
- Changes to the existing GoalsPanel CRUD UI — stays as-is
- Changes to file-based goal detection (`POST /goals/detect`) — stays as-is
- Auto-mode (GSD's `--auto` flag) — future enhancement
