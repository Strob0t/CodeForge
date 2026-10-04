# Autonomous goal benchmark: can an agent build a real program on its own?

**Status:** defined 2026-10-03; acceptance suite (80 cases) and grader ready 2026-10-04 and validated against a reference solution (100 %) and two weak variants; no run yet.
**Owner request (2026-10-03):** test whether the models reach a real programming goal completely on their own, through
CodeForge, with good code quality, and record what we learn.

This benchmark complements the [autonomous goal-to-program test plan](autonomous-goal-to-program-testplan.md)
(scenarios S1 to S4). Those runs check behaviour with checks the agent could also run itself. This one adds three
things:
- a **hidden acceptance suite**, written before the run and never visible to the agent;
- a **code-quality rubric**;
- an **intervention count**: every human action after the goal is given counts against the run.

## The goal

[`testdata/autonomous-goal/mdlinkcheck/SPEC.md`](../../testdata/autonomous-goal/mdlinkcheck/SPEC.md):
`mdlinkcheck`, a Python CLI that finds broken links in Markdown files. It was chosen for four reasons:
- **Realistic and mid-sized:** a parser, path and anchor resolution, GitHub heading slugs, configuration, two output
  formats, exit codes and an optional network check. A strong model needs a few hundred lines and a solid test suite.
- **Precisely testable:** every behaviour is fixed by the spec (positions, sorting, JSON shape, exit codes), so a
  hidden suite can test it from the outside, through the command line.
- **Deterministic:** the network check is tested against a local HTTP server, and nothing else needs the internet.
- **Language:** Python, which every model knows well. A TypeScript variant can follow.

## Run procedure (through CodeForge, no manual help)

1. **Workspace:** a new git repository containing only a one-line `README.md` (`# mdlinkcheck`) and an empty
   commit history. Nothing from the spec, nothing from the acceptance suite.
2. **Project:** `POST /api/v1/projects` with `local_path` set to the workspace, and the config
   `{"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount", "test_command": "pytest -q", "lint_command": "ruff check ."}`.
   The project gets the model under test, with no fallbacks.
3. **Goal:** `POST /api/v1/projects/{id}/goals` with the spec text as the goal content (kind `requirement`).
4. **Plan:** let CodeForge derive the roadmap and features from the goal (AI roadmap / `propose_roadmap`), unchanged.
5. **Execute:** start the auto-agent on the roadmap at autonomy level 4, with approvals bypassed for this conversation.
   Let it run until it reports done, a budget or step limit ends it, or 4 hours of wall time pass.
6. **No help.** Any of these counts as an **intervention**, and is logged with the time and the reason:
   - a message after the goal;
   - an approval or a restart;
   - an edit in the workspace;
   - a configuration change during the run.
   A run with interventions is reported, but it does not count as autonomous.
7. **Freeze:** after the run, copy the workspace and the CodeForge run data (trajectory, costs, tool-call counts). Then
   grade the copy.

The acceptance suite lives outside the workspace (`testdata/autonomous-goal/mdlinkcheck/acceptance/`). Run the
benchmark with tool isolation on (`CODEFORGE_TOOL_ISOLATION=required`): Landlock then confines the agent to its
workspace, so it cannot read the suite. If a run uses development mode without isolation, the report says so.

## Grading

The grader (`testdata/autonomous-goal/mdlinkcheck/grade.py`) installs the result with `pip install -e .` in a fresh
virtual environment, then runs every check below on the frozen copy. It prints a JSON report.

| Check | Tool | Weight | Full score when |
|---|---|---|---|
| Acceptance | hidden pytest suite, through the console script | 40 % | 100 % of the cases pass; the score is proportional |
| Own tests | the project's pytest suite | 10 % | all pass, at least 20 tests |
| Coverage | pytest-cov, branch coverage of `src/` | 10 % | at least 85 %; 0 below 50 %, linear in between |
| Lint | `ruff check` with rules E, F, W, I, B, UP, SIM, RUF | 10 % | no findings; minus 1 point per finding |
| Types | `mypy --strict src/` | 10 % | no errors; minus 1 point per error |
| Complexity | radon cyclomatic complexity | 5 % | no function above 10; minus 1 point per function above 10 |
| Security | bandit | 5 % | no medium or high findings |
| Review | LLM judge with a fixed rubric (structure, naming, error handling, tests, README), 1 to 10, average of 3 runs | 10 % | 10 |

Point deductions are applied to the check's own 10-point scale, then weighted. A run **succeeds** when it is
autonomous (0 interventions), the acceptance score is at least 90 %, and the total is at least 70 %.

### Grader rules (2026-10-04)

The table above leaves some choices open; `grade.py` makes them as follows:
- **Own tests:** 10 x passed/executed x min(1, executed/20); skipped tests count neither way.
- **Coverage:** coverage.py's combined line-and-branch total of `src/`, subprocesses included.
- **Lint:** `ruff check --isolated` with the rules above; only the project's `line-length` is honoured, kept between 79
  and 120 (default 88).
- **Types:** `mypy --strict --config-file=` (the project's own mypy config is ignored).
- **Complexity:** functions, methods and closures count; a file radon cannot parse costs 1 point.
- **Security:** bandit with an empty `--ini`; minus 5 points per medium or high finding.
- **Project configuration cannot hide findings:** coverage, radon and bandit use the grader's own configuration, and the
  raw results count `noqa`, `type: ignore` and `nosec` comments.
- **No source:** an empty `src/` scores 0 on all four static checks.
- **Review:** the mean of five criteria over 3 valid judge runs (failed runs are retried, at most 6 calls). Without
  `--judge-model` the review is "not scored" and the total is scaled over the scored checks.
- **Interventions** are passed in with `--interventions N`.

Quality is 60 % of the weight, so a well-made but incomplete program can still reach a high total: a validation variant
without anchors and JSON output scored 90 % overall with 77.5 % acceptance and fails only through the "acceptance at
least 90 %" rule. Read the acceptance score first.

### Validation (2026-10-04)

| Variant | Acceptance | Own tests | Coverage | Lint | Types | Complexity | Security | Total (review not scored) |
|---|---|---|---|---|---|---|---|---|
| Reference solution (kept outside the repository) | 80/80 | 98/98 | 99.9 % | 0 | 0 | 0 over 10 | 0 | 100 %, success |
| Weak: no anchors, no JSON | 62/80 | 43/43 | 86.9 % | 0 | 0 | 0 | 0 | 90.0 %, not a success |
| Sloppy single module | 48/80 | 5/5 | 49.2 % | 13 | 9 | 3 over 10 | 1 medium | 37.2 %, not a success |

A mutation check made 22 small deliberate spec violations in a copy of the reference (ASCII-only slugs,
case-insensitive fragments, no inline-code masking, excludes relative to the working directory, no GET after 405,
unsorted output, ...); each one fails at least one acceptance case.

### Running the grader

```bash
python3.12 -m venv ~/.venvs/mdlinkcheck-grade
~/.venvs/mdlinkcheck-grade/bin/pip install pytest pytest-cov ruff mypy radon bandit
~/.venvs/mdlinkcheck-grade/bin/python testdata/autonomous-goal/mdlinkcheck/grade.py /path/to/frozen-workspace \
    --out report.json [--interventions N] [--judge-model MODEL]
```

The judge needs `LITELLM_BASE_URL` and `LITELLM_MASTER_KEY`. The JSON report goes to stdout (and `--out`), a summary
table to stderr. Exit codes: 0 success, 1 graded but not a success, 2 the grader could not run. The grader works on a
copy in the system temp directory (or `--work-dir`) and removes it unless `--keep` is given. Details:
[`testdata/autonomous-goal/mdlinkcheck/README.md`](../../testdata/autonomous-goal/mdlinkcheck/README.md).

## Metrics per run

- Model, provider, quantisation and hardware (local models: CPU or GPU, threads).
- Success (yes or no), the total score and each check.
- Wall time, LLM calls, tool calls by tool, steps, tokens in and out, and cost.
- Interventions, each with the time and the reason.
- What the plan looked like (roadmap features) and how far the agent got.
- The failure mode, when it failed: what broke first, and whether CodeForge or the model caused it.

## Runs

| Date | Model | Hardware | Autonomous | Acceptance | Total | Wall time | Cost | Report |
|---|---|---|---|---|---|---|---|---|
| - | `ollama/qwen3:4b-instruct` | 4 CPU cores | planned | | | | | |
| - | a cloud model (owner provides the key as an environment secret) | - | planned | | | | | |

The lessons from each run go into [live-e2e-findings.md](live-e2e-findings.md), and the bugs into
[docs/todo.md](../todo.md#known-issues).
