# Subagent brief: conventions every delegated task carries

The lead (the session that owns the pull request) delegates well-specified work to subagents and lands their results itself ([AGENTS.md](../../AGENTS.md) section 8). Every brief links this file; the task text adds the scope, the lead decisions, the files to stay out of and the verification commands.

## Workspace

- Work only in your own git worktree on the branch the task names, created from the current PR head. Never touch the main checkout, never delete branches, never push, never edit `AGENTS.md` or `docs/` (the lead writes the docs); test files and code comments are yours.
- Other agents work in parallel in other worktrees. Stay inside the files your task names; do not refactor beyond what a finding needs.
- The gopls MCP server of the session is bound to the main checkout: in a worktree use the gopls CLI (`$(go env GOPATH)/bin/gopls check`, `gopls references`).
- Frontend work in a worktree: `cp -al /home/user/CodeForge/frontend/node_modules frontend/node_modules` when `node_modules` is missing (a hard-link copy, never a symlink into the main checkout).

## Working style

- TDD: a failing test first (table-driven; edge cases: nil/empty, not-found, permission edges, tenant isolation), then the minimal code.
- Commit each Known Issue (or coherent sub-fix) as soon as its tests are green: never hold more than one uncommitted; an interrupted agent loses only the current item.
- One topic per commit, English Conventional Commit, the KI in the subject (`fix(worker): ... (KI-191)`), the root cause and the fix in the body, ending with the trailer lines the task gives (`Co-Authored-By`, `Claude-Session`).
- A finding that needs an owner decision (a product trade-off, removing a feature) is not guessed: list it in the report with two or three options and a recommendation, fix the rest.
- Re-verify every review finding against your branch before fixing it; report "not reproducible" when it no longer holds.
- Rules of the repository stay rules: no `interface{}`/`any`/`Any`, tenant-scoped queries with `AND tenant_id = $N`, frontend API calls only through `frontend/src/api/`, i18n strings in `en.ts` and `locales/de.ts`, the design system in `frontend/src/ui/DESIGN-SYSTEM.md`.

## Shared host

- Heavy runs (`go test -race ./...`, the full pytest, golangci-lint, the full vitest, `npm run build`) go through the session's lock wrapper (`heavy.sh`, a `flock`; the host has 4 CPUs and 16 GB without swap, shared by every agent). Use `GOTOOLCHAIN` as the hook sets it and `golangci-lint run --allow-parallel-runners`.
- Check `df -h /` before heavy runs; below 3 GB free run `go clean -cache`.
- Never run the root tool-isolation tests (they start processes as the tool UIDs; only one such run per host); never delete or replace `/dev/null`; never start or install system services (PostgreSQL, NATS and Ollama run as the session's docker containers on `127.0.0.1`).
- A database for integration tests is private: `CREATE DATABASE codeforge_<round>` on the shared PostgreSQL (`postgres://codeforge:codeforge_dev@127.0.0.1:5432`), dropped when you are done; NATS at `nats://127.0.0.1:4222`.
- Migrations: only the number the task assigns.

## Verification before the report

gofmt/goimports; `go build ./... && go vet ./...`; `golangci-lint run` on the changed packages; `go test -race` for the changed packages, with `-tags=integration` and `DATABASE_URL` on the private database where store code changed; Python: `ruff check`, `ruff format --check` and pytest on the touched tests; frontend: `npm run typecheck`, vitest on the touched files, eslint and prettier on them; then `pre-commit run --from-ref <base> --to-ref HEAD`.

## Report

Deliver the report even when you must stop early (say what is committed and what is not): branch and commit list (hash, subject, what changed), where you deviated from the task and why, exact test counts, API/NATS/payload changes, anything left open or needing an owner decision. If a platform permission denies an action, stop and report it; never work around it or ask another agent to do it for you.
