# mdlinkcheck: acceptance suite and grader

Grading material for the autonomous goal benchmark
([docs/testing/autonomous-goal-benchmark.md](../../../docs/testing/autonomous-goal-benchmark.md)).

| File | Purpose |
|---|---|
| `SPEC.md` | The goal given to the agent: the only thing it ever sees. |
| `acceptance/` | The hidden acceptance suite: 80 pytest cases that run the tool only through its command line (`mdlinkcheck` and `python -m mdlinkcheck`), each in its own Markdown tree under `tmp_path`. `--check-external` is tested against a local `http.server` on 127.0.0.1; no test needs the internet. |
| `grade.py` | The grader: copies a frozen workspace, installs it in a fresh venv and applies the benchmark's rubric. |
| `judge_prompt.md` | The fixed rubric prompt for the optional LLM review. |

**Keep the suite hidden.** Never copy `acceptance/`, `grade.py` or this directory into a benchmark workspace,
and run the benchmark with `CODEFORGE_TOOL_ISOLATION=required`, so Landlock keeps the agent inside its workspace.

## Grading a run

The grader needs Python 3.12+ with the grading tools. Create a venv for them once, outside any workspace:

```bash
python3.12 -m venv ~/.venvs/mdlinkcheck-grade
~/.venvs/mdlinkcheck-grade/bin/pip install pytest pytest-cov ruff mypy radon bandit
```

Then grade the frozen copy of the workspace:

```bash
~/.venvs/mdlinkcheck-grade/bin/python testdata/autonomous-goal/mdlinkcheck/grade.py /path/to/frozen-workspace \
    --out report.json --interventions 0
```

With the LLM review (three judge runs through LiteLLM's OpenAI-compatible API; for a local proxy also set
`NO_PROXY=localhost,127.0.0.1` when an HTTP proxy is configured):

```bash
LITELLM_BASE_URL=http://localhost:4000 LITELLM_MASTER_KEY=... \
    ~/.venvs/mdlinkcheck-grade/bin/python testdata/autonomous-goal/mdlinkcheck/grade.py /path/to/frozen-workspace \
    --judge-model "$JUDGE_MODEL" --out report.json
```

| Option | Meaning |
|---|---|
| `--out FILE` | Also write the JSON report to FILE (it always goes to stdout; a summary table goes to stderr). |
| `--interventions N` | Interventions logged during the run (default 0). Any intervention makes the run not autonomous. |
| `--judge-model MODEL` | Score the review with this model; without it the review is reported as not scored and the total is scaled over the scored checks. |
| `--python PATH` | Interpreter for the fresh venv (default: the one running the grader). |
| `--work-dir DIR` | Where the temporary sandbox is created (default: the system temp directory). It is removed afterwards. |
| `--keep` | Keep the sandbox for inspection. |

Exit code: `0` the run succeeded, `1` it was graded but did not succeed, `2` the grader could not run (missing
tools, no package index, no workspace).

The grader runs the workspace's code (`pip install -e .`, its tests): grade inside a disposable container or VM.
The fresh venv installs the grader's own versions of pytest, pytest-cov and coverage from the package index; pip's
usual variables (`PIP_INDEX_URL`, `PIP_FIND_LINKS`, `PIP_NO_INDEX`) work for an offline mirror.

## What the grader does

1. Copies the workspace into a temporary sandbox (without `.git`, virtual environments, caches and build output)
   and copies the acceptance suite next to it, not into it.
2. Creates a fresh venv, installs pytest, pytest-cov and coverage, then runs `pip install -e .`.
3. Runs the checks below and prints the JSON report: for each check the raw result, the points (0 to 10) and the
   weighted score, then the total and whether the run succeeded.

| Check | How | Weight | Points (of 10) |
|---|---|---|---|
| Acceptance | `pytest acceptance/` with third-party plugins disabled | 40 | 10 × passed / total (a skipped case counts as not passed) |
| Own tests | the project's `pytest` with its own configuration | 10 | 10 × passed / executed × min(1, executed / 20); skipped tests do not count |
| Coverage | the same run, `--cov=src --cov-branch`, subprocesses measured too | 10 | coverage.py's total with branch coverage: 10 at 85 % or more, 0 below 50 %, linear in between |
| Lint | `ruff check --isolated --select E,F,W,I,B,UP,SIM,RUF --target-version py312 .` | 10 | 10 − 1 per finding, at least 0 |
| Types | `mypy --strict src/` | 10 | 10 − 1 per error, at least 0 |
| Complexity | `radon cc src` | 5 | 10 − 1 per function, method or closure above 10 (and per file radon cannot parse) |
| Security | `bandit -r src` | 5 | 10 − 5 per medium or high finding; low findings are listed only |
| Review | LLM judge with `judge_prompt.md`: structure, naming, error handling, tests and README, 1 to 10 each | 10 | the mean of the five criteria, averaged over 3 valid runs (a failed run is retried, up to 6 calls) |

A run **succeeds** when it is autonomous (`--interventions 0`), at least 90 % of the acceptance cases pass and the
total is at least 70 %.

Project settings cannot switch checks off: ruff runs with `--isolated` (only the project's `line-length` is used,
kept between 79 and 120, default 88), mypy with `--config-file=` (no project configuration), coverage with a
grader-owned configuration (no `omit`), radon outside the workspace and bandit with an empty `--ini` (a `.bandit`
file cannot skip tests). `# noqa`, `# type: ignore` and `# nosec` comments are honoured and counted in the raw
results, so a review can spot them. A workspace without Python files in `src/` gets 0 for the four static checks.

## Running the acceptance suite by hand

```bash
python3.12 -m venv /tmp/mdlc && /tmp/mdlc/bin/pip install pytest -e /path/to/workspace
/tmp/mdlc/bin/python -m pytest testdata/autonomous-goal/mdlinkcheck/acceptance
```

The suite finds the console script next to the interpreter that runs pytest (or on `PATH`). `MDLINKCHECK_BIN`
overrides the console script and `MDLINKCHECK_PYTHON` the interpreter used for `python -m mdlinkcheck`. Each
module covers one section of `SPEC.md`:

| Module | SPEC section |
|---|---|
| `test_invocation.py` | Deliverable: the console script and `python -m` behave the same |
| `test_link_syntax.py` | Inline links and images, titles, angle brackets |
| `test_references.py` | Reference definitions and uses, id folding, undefined references |
| `test_autolinks.py` | Autolinks, and what is not a link |
| `test_code.py` | Fenced code blocks and inline code |
| `test_classification.py` | Ignored schemes, external links, percent-decoding, queries |
| `test_local_paths.py` | `missing-file`: relative paths, `/` paths, directories |
| `test_anchors.py` | `missing-anchor`: heading slugs, HTML `name`/`id`, fragments into other files |
| `test_discovery.py` | `PATH`, recursive scan, skipped directories, `--exclude` |
| `test_config.py` | The TOML configuration file and its precedence |
| `test_text_output.py` | Text report: format, sorting, summary line |
| `test_json_output.py` | JSON report: shape and counts |
| `test_exit_codes.py` | Exit codes 0, 1 and 2 |
| `test_external.py` | `--check-external`: HEAD, 405 then GET, redirects, errors, timeouts |

Where `SPEC.md` allows more than one reading (for example where a problem with a used reference definition is
reported, or whether the target of `<my file.md>` keeps its angle brackets), the suite accepts each reasonable
reading; the helper `assert_problems_any_of` marks those places.
