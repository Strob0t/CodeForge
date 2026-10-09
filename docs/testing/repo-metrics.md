# Repository process metrics

`scripts/repo-metrics.sh [base-ref]` measures how the rules of `AGENTS.md` are kept on the branch since its merge-base with `staging`: commit sizes, whether fixes carry tests and features carry docs, how many product files exceed the 700-line ceiling (`scripts/check-file-sizes.sh`; i18n catalogs and `frontend/src/api/types.ts` are exempt as data and contract files), and the frontend lint warnings. Re-run it at the end of every fix wave and append a row; the first row is the baseline measured after the MasterSelects review of 2026-10-09 (owner decision: the stock is fixed, every ceiling starts at zero).

| Date | Range | Commits | Avg files / lines per commit | Commits > 400 lines | Commits > 1,000 lines | Fix commits with a test | Feat commits with docs | Files > 700 lines (Go / Python / TS) | Frontend warnings |
|---|---|---|---|---|---|---|---|---|---|
| 2026-10-09 | `cb9b63ce..d29215e4` (PR branch) | 758 | 6.8 / +265 -45 | 167 | 47 | 505 / 546 (92 %) | 4 / 52 (8 %) | 12 / 11 / 6 | 22 |

Reading the baseline:

- The 47 commits over 1,000 lines are the big rounds (KI-96 tool identities, KI-85 webhooks, KI-71 tool users, KI-118 backend CLIs, the acceptance suite); the rule "one topic, under 400 product lines" applies from now on and the number must not grow except for generated files and migrations.
- Features show 8 % docs co-change because the lead commits the docs of a landed round separately (`docs: ...`), as AGENTS.md section 8 prescribes; the rule "docs ship with the code" therefore holds per round, not per commit. Fix commits carry tests in 92 % of cases.
- The 29 files over 700 lines are listed by `scripts/check-file-sizes.sh`; the split rounds are tracked in `docs/todo.md` (process improvements of 2026-10-09). The check joins pre-commit once the stock is split.
