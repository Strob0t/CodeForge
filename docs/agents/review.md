# Review brief: read-only snapshot reviews

Every round a subagent delivers is reviewed before the lead lands it ([AGENTS.md](../../AGENTS.md) section 8). The reviewer is a fresh agent that reads a fixed snapshot and changes nothing.

## Setup

- In an own worktree: `git checkout --detach <head of the reviewed branch>`. No edits, no commits, no pushes, no services started or installed.
- Allowed runs: `go vet`, targeted `go test -race ./<pkg>/ -run <Name>`, vitest on single files, ruff/pytest on single files. Nothing heavier; the lead runs the full verification when landing.
- Read first: `AGENTS.md` (the rules the round claims to follow), the `docs/todo.md` entries of its Known Issues, the audit findings it references, and the round's own description (`git log --stat <base>..<head>`).

## What to review, in this order

1. Security and isolation: authorization bypasses, tenant isolation (`tenant_id` scoping, `withPayloadTenant`), credentials and outbound hosts, workspace safety (`internal/git`, `workspacefs`, `tool_process.py`).
2. Delivery semantics (ADR-016): at-most-once starts, completion claims, double or lost completions, heartbeats and the DLQ.
3. Correctness of the claimed fix: re-derive the failure scenario from the code path; check boundary values and the error paths; check every caller of a changed signature, including test mocks.
4. Regressions for the default single-user installation (the admin of the default tenant, `auth.enabled=false`, the internal key).
5. Tests: do they assert the behaviour (403, 404, the exact payload), or pass vacuously? Would the test have failed before the fix?

Skip style remarks; the linters own those.

## Finding format

One finding per item, most severe first:

```
N. SEVERITY (HIGH | MEDIUM | LOW), CONFIRMED | PLAUSIBLE — one-line title
   Where: path/file.go:line (snapshot <sha>)
   What breaks: the concrete scenario (input or state -> wrong outcome)
   Fix: the smallest change that closes it
```

`CONFIRMED` means the code path or a test reproduces it; `PLAUSIBLE` means the mechanism is real but the trigger was not reproduced. Then a short **Checked OK** list of what was verified and holds, so the lead knows what was covered.

The lead decides each finding (fix round, accept with a Known Issue, or reject with a reason) and writes the docs; the reviewer never applies fixes.
