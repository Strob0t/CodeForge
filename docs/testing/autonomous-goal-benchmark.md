# Autonomous goal benchmark: can an agent build a real program on its own?

**Status:** defined 2026-10-03; acceptance suite and grader in preparation; no run yet.
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
