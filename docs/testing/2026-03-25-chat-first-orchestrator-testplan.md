# Chat-First Orchestrator — Integration Test Plan

**Date:** 2026-03-25
**Feature:** Chat-First Orchestrator (Phase 33)

## Prerequisites
- Full stack running (Docker services, Go backend, Python worker, Frontend)
- At least one LLM model available via LiteLLM

## Test Scenarios

### S1: Auto Goal Extraction
1. Open project chat
2. Type: "I want to build a REST API with JWT auth and PostgreSQL"
3. Verify: GoalProposalCards appear in chat WITHOUT clicking "AI Discover"
4. Verify: Cards show correct kinds (vision/requirement/constraint)

### S2: Goal Approval Flow
1. Approve 2+ goals from S1
2. Verify: Goals appear in GoalsPanel (Plan view)
3. Verify: Orchestrator suggests roadmap generation

### S3: Roadmap Generation
1. Confirm roadmap generation in chat
2. Verify: RoadmapProposalCards appear with milestones and atomic steps
3. Verify: Complexity badges shown (trivial/simple/medium/complex)
4. Verify: Model tier indicators shown (weak/mid/strong)
5. Approve all -> verify RoadmapPanel updates

### S4: Deep-Link (Goal)
1. In Plan view, click "Discuss" on a goal
2. Verify: Chat input fills with `[goal:ID]` reference
3. Verify: On mobile, view switches to chat

### S5: Deep-Link (Roadmap Step)
1. In Plan view, click "Discuss" on a roadmap step
2. Verify: Chat input fills with `[roadmap-step:ID]` reference

### S6: Panel Consolidation
1. Open panel dropdown
2. Verify: Only 4 entries (Plan, Execute, Code, Govern)
3. Select "Plan" -> verify Goals + Roadmap + FeatureMap visible
4. Verify: Sections are collapsible

### S7: Sub-Agent Spawn
1. During execution, verify sub-agent request appears as text message in chat
2. Verify: Log shows "sub-agent requested" with role and task
