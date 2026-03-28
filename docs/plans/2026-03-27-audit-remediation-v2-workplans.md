# Audit Remediation v2 — 20 Worktree Plans (2026-03-27)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate 63 findings from the 2026-03-27 universal audit (filtered: no .env/docker-compose) across 20 independent worktrees.

**Architecture:** Each WT is an isolated git worktree at `.worktrees/<name>`, branching from `staging`. WTs designed to minimize file overlap — merge in priority order.

**Source Audit:** `docs/audits/2026-03-27-universal-audit-report.md`

**Research:** 6 specialist agents researched GitHub repos, academic papers, and industry best practices. Findings integrated per-WT below.

---

## Execution Order & Dependencies

```
Phase 1 (parallel, no deps):  WT-1  WT-2  WT-3  WT-4  WT-5  WT-7  WT-8  WT-12  WT-16  WT-17  WT-18  WT-19
Phase 2 (after Phase 1):      WT-6  WT-9  WT-10  WT-11  WT-13  WT-14  WT-15
Phase 3 (after ALL others):   WT-20
```

WT-20 (ISP+DI) touches all 48 service files — **must go last**.

---

## Finding Coverage Matrix

| WT | Branch | Findings | Phase | Effort |
|----|--------|----------|-------|--------|
| 1 | `fix/security-credentials` | SEC-002, SEC-008, COMP-004, COMP-013 | 1 | S |
| 2 | `fix/security-code-safety` | SEC-005, SEC-006, SEC-012 | 1 | S |
| 3 | `fix/rbac-audit-logging` | ARCH-005, SEC-009, INFRA-013, QUAL-019 | 1 | S |
| 4 | `fix/gdpr-consent-tenant` | COMP-001, COMP-002, COMP-011 | 1 | L |
| 5 | `fix/gdpr-privacy-encryption` | COMP-006, COMP-007, COMP-010 | 1 | M |
| 6 | `fix/go-type-safety` | QUAL-002, QUAL-015, QUAL-016 | 2 | L |
| 7 | `fix/python-test-coverage` | QUAL-001 | 1 | L |
| 8 | `fix/go-test-coverage` | QUAL-004 | 1 | L |
| 9 | `fix/python-quality-decomp` | QUAL-008..011, QUAL-017..018, ARCH-004, ARCH-014 | 2 | XL |
| 10 | `fix/runtime-refactoring` | QUAL-007, QUAL-012..014 | 2 | L |
| 11 | `fix/go-dry-webhooks` | QUAL-003, QUAL-005, QUAL-006, ARCH-013, QUAL-020 | 2 | M |
| 12 | `fix/frontend-a11y-errors` | QUAL-010, COMP-008, COMP-009, COMP-012 | 1 | M |
| 13 | `fix/handlers-decomposition` | ARCH-003, ARCH-006..009 | 2 | XL |
| 14 | `fix/arch-code-organization` | ARCH-010, ARCH-015 | 2 | M |
| 15 | `fix/frontend-types-hooks` | ARCH-011, ARCH-012 | 2 | M |
| 16 | `fix/nats-ci-hardening` | INFRA-006, INFRA-010..011, SEC-010 | 1 | M |
| 17 | `fix/http-timeout-db-limits` | INFRA-003, INFRA-004 | 1 | M |
| 18 | `fix/ops-backup-health` | INFRA-005, INFRA-014, INFRA-015 | 1 | S |
| 19 | `fix/openapi-docs` | COMP-003, COMP-014, COMP-015 | 1 | L |
| 20 | `fix/isp-di-reform` | ARCH-001, ARCH-002 | 3 | XXL |

**Effort:** S=1-2h, M=2-4h, L=4-8h, XL=8-16h, XXL=16-32h

---

*Full atomic task breakdowns for each WT follow. See the research summary at the end for sources.*

*Plan document: `docs/plans/2026-03-27-audit-remediation-v2-workplans.md`*
*Audit report: `docs/audits/2026-03-27-universal-audit-report.md`*
