# Full code review (2026-10-06)

**Owner request:** a complete code review of CodeForge. **Snapshot:** `71bdf542` (PR head on 2026-10-06). **Method:** twelve read-only review agents, one per area, each with the AGENTS.md rules and the Known Issues list, so known open issues were not reported again. Each finding has a severity, a concrete failure scenario and a verdict: CONFIRMED (traced end to end or reproduced with a scratch test) or PLAUSIBLE. The area reports are kept as written in this folder.

## Result

- **211 findings** in 12 areas, consolidated into **53 Known Issues** (KI-171..KI-223): 10 high, 35 medium, 8 low. Findings that several reviewers found independently were merged; every finding maps to exactly one Known Issue (the `R<area>-<n>` references).
- **No critical finding.** The Go/Python NATS contract (62 subjects, all payload fields), tool isolation, WebSocket tickets, JWT handling, webhook signatures, the markdown renderer (no XSS) and the migration round trip were checked and found sound.
- **Main themes:**
  - **Agent quality on small models:** unconditional prompt reminders (KI-190), a stall detector that aborts normal edit/test loops (KI-191), and tool selection and routing based on the first user message (KI-192). These likely explain much of benchmark run 1 (KI-153, KI-160).
  - **Authorization:** viewers can start agent runs and write tenant data (KI-171), and the audit log is not wired in production (KI-172).
  - **The Go Core trusts agent-writable git state:** a FIFO hangs every git call (KI-187), and delivery follows agent-set push config (KI-188).
  - **Single-user operations:** the production WAL archive fills the disk within weeks (KI-210), and the restore script can drop the database (KI-212).
  - **Silent data loss:** roadmap "Sync to file" destroys spec files (KI-203), and streamed LLM calls are costed at $0, so budgets never stop a run (KI-196).

## Areas

| Area | Scope | Findings | Report |
|---|---|---|---|
| R1 | Go HTTP layer, cmd wiring, config | 22 | [R1.md](R1.md) |
| R2 | Go agent-execution services | 16 | [R2.md](R2.md) |
| R3 | Go platform services | 17 | [R3.md](R3.md) |
| R4 | Go context, knowledge, roadmap, benchmark, A2A, LSP services | 14 | [R4.md](R4.md) |
| R5 | PostgreSQL store and migrations | 20 | [R5.md](R5.md) |
| R6 | Go adapters, internal/git, workspacefs, netutil | 15 | [R6.md](R6.md) |
| R7 | Domain, ports, Go/Python NATS contract | 4 | [R7.md](R7.md) |
| R8 | Python worker core | 19 | [R8.md](R8.md) |
| R9 | Python worker modules | 19 | [R9.md](R9.md) |
| R10a | Frontend part 1 | 23 | [R10a.md](R10a.md) |
| R10b | Frontend part 2 | 22 | [R10b.md](R10b.md) |
| R11 | Infrastructure, CI, scripts | 20 | [R11.md](R11.md) |

## Known Issues and fix rounds

Priority follows the audience rule (self-hosters and single users first).

- **Wave 1:** S10-E, S10-I, S10-D, S10-A, S10-H, S10-G.
- **Wave 2:** S10-F, S10-C, S10-B.
- **Wave 3:** S10-J, S10-K.

Details per issue are in [`docs/todo.md`](../../todo.md#known-issues).

### S10-A: HTTP authorization and audit (Go)

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-171 | high | Viewers can start agent runs and change tenant data | R1-2, R2-1, R3-1, R3-9, R10a-1, R10b-6, R4-10 |
| KI-172 | high | The audit log is not wired in production | R1-1 |
| KI-173 | medium | Editors can delete projects through the batch route | R1-3, R3-6, R10a-2 |
| KI-174 | medium | Tenant admins can list and change every tenant; disabling a tenant has no effect | R1-4, R3-5 |
| KI-175 | medium | API key scopes are never enforced | R1-8, R3-3 |
| KI-176 | low | HTTP leftovers: cross-tenant user lookups, A2A push tokens, body limits, 500s | R1-14, R3-8, R1-15, R10a-22, R1-16, R1-21 |

### S10-B: Authentication and sessions (Go, frontend)

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-177 | medium | Login and password changes write back stale user rows | R3-2 |
| KI-178 | medium | Session handling: logout fails open, parallel refreshes log users out | R10a-3, R10a-4, R10a-15, R10a-16 |
| KI-179 | low | Auth leftovers | R1-13, R3-11, R3-15, R3-12, R5-18, R3-16, R10b-4, R1-11 |

### S10-C: Secrets, SSRF and tenant mixing

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-180 | medium | Project credentials in the project config are readable by every user | R3-4, R6-3 |
| KI-181 | medium | `GET /projects/remote-branches` runs `git ls-remote` against any host | R1-17, R3-10, R6-5 |
| KI-182 | medium | The built-in MCP server is unauthenticated by default | R1-10, R6-4 |
| KI-183 | medium | Agent memories take the tenant from the request body and recall has no tenant filter | R4-5, R8-4, R9-3 |
| KI-184 | medium | Routing state mixes tenants and is poisoned by personal keys | R1-5, R3-7, R9-4, R9-5 |
| KI-185 | medium | Custom modes and pipeline templates are global and in memory | R1-9, R2-6, R10b-1 |
| KI-186 | medium | LSP routes have no tenant check and start/stop races (when `lsp.enabled`) | R1-12, R4-7 |

### S10-D: Git and workspace safety in the Go Core

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-187 | high | A FIFO in the workspace hangs Go Core git calls | R6-1, R6-6 |
| KI-188 | medium | Delivery pushes where the agent's git config says, and push failures count as delivered | R6-2, R2-5 |
| KI-189 | low | Git and VCS leftovers | R6-13, R6-15, R6-11, R6-12, R3-14 |

### S10-E: Agent loop and LLM client (worker)

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-190 | high | Prompt reminders are rendered into every turn unconditionally | R4-1 |
| KI-191 | high | The stall detector aborts normal edit/test loops | R8-1 |
| KI-192 | medium | Tool selection and routing use the wrong input and skip the policy | R8-2, R8-9, R8-10, R9-6 |
| KI-193 | medium | An agentic conversation without a workspace runs file tools in the worker's own directory | R8-3 |
| KI-194 | medium | Tool processes survive Stop, timeout and abort | R8-6, R8-7, R9-8, R8-18 |
| KI-195 | medium | Multi-rollout conversations stash user changes and keep the last rollout | R8-5 |
| KI-196 | high | Streamed LLM calls cost $0 and mid-stream errors complete the turn | R9-1, R9-2, R9-12, R9-7 |
| KI-223 | medium | The owner-run walk runs out of file descriptors on wide trees | R8-8 |

### S10-F: Store, A2A and data lifecycle

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-197 | high | The A2A protocol path is broken in several places | R5-1, R7-2, R4-9, R4-4, R8-11, R6-10, R4-6, R5-8 |
| KI-198 | medium | Cursor pagination is wrong for trajectories, audit and channels | R5-2, R5-3, R10a-7, R5-20, R10a-13 |
| KI-199 | medium | Inserts accept another tenant's project ID | R5-4 |
| KI-200 | medium | Deleted projects and erased users leave data behind | R5-6, R5-5, R5-10 |
| KI-201 | medium | System jobs see only the default tenant; review policies never run | R5-7, R4-11, R2-4, R5-15 |
| KI-202 | low | Store leftovers | R5-11, R5-12, R5-13, R5-14, R5-17, R5-9, R5-16, R5-19 |

### S10-G: Roadmap sync, trust and branch protection

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-203 | high | Roadmap sync destroys spec files and import loses status | R4-2, R4-3, R4-12 |
| KI-204 | medium | An unknown trust minimum fails open | R7-1 |
| KI-205 | medium | Branch protection rules are never evaluated | R7-3, R2-10 |

### S10-H: Agent runtime state (Go)

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-206 | high | Auto-agent Stop leaves the status at "stopping" for good | R2-2 |
| KI-207 | medium | Phantom running turns and orphaned conversation runs | R2-3, R2-14, R2-15, R10a-14 |
| KI-208 | medium | Orchestrator and run bookkeeping defects | R2-7, R2-13, R2-12, R2-11, R2-16 |
| KI-209 | medium | `/compact` and goal proposals do not work as documented | R2-8, R8-13, R2-9, R10b-5 |

### S10-I: Operations and deployment

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-210 | high | The production WAL archive grows without bound | R11-1 |
| KI-211 | medium | Production shares one login rate-limit bucket across all clients | R11-2, R1-7 |
| KI-212 | medium | `restore-postgres.sh latest` can drop the database and restore a random file | R11-3 |
| KI-213 | medium | Go Core network and resilience defaults | R11-4, R11-10, R1-6, R6-8, R6-7, R6-9, R1-18, R1-19, R1-20, R1-22 |
| KI-214 | medium | Build, CI and deployment hygiene | R11-11, R11-5, R11-6, R11-7, R11-8, R11-9, R11-12, R11-13, R11-14, R11-15, R11-16, R11-17, R11-18, R11-19, R11-20 |

### S10-J: Frontend

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-215 | medium | One failed request replaces the whole app with the error screen | R10a-5, R10a-6 |
| KI-216 | medium | Chat UI state defects | R10b-9, R10b-7, R10b-16, R10b-15, R10a-21, R10b-10, R10b-20 |
| KI-217 | medium | File panel defects | R10b-11, R10b-12, R10b-13 |
| KI-218 | medium | Settings and admin page defects | R10b-2, R10b-21, R10b-3, R10b-14 |
| KI-219 | low | Frontend leftovers | R10a-9, R10a-8, R10a-10, R10a-11, R10b-8, R10a-12, R10a-17, R10a-18, R10a-20, R10a-23, R10b-17, R10b-18, R10b-19, R10b-22 |

### S10-K: Evaluation, prompt evolution and leftovers

| KI | Severity | Issue | Findings |
|---|---|---|---|
| KI-220 | low | Evaluation and benchmark defects | R9-9, R9-10, R9-11, R9-18, R8-12, R10a-19, R8-16 |
| KI-221 | low | Prompt evolution, prompt sections and routing helpers are not wired | R4-8, R4-13, R9-13, R8-19, R9-14, R9-15, R9-16, R9-17 |
| KI-222 | low | Worker and service leftovers | R8-14, R8-15, R8-17, R7-4, R4-14, R6-14, R9-19, R3-13, R3-17 |
