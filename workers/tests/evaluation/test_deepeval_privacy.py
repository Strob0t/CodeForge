"""KI-54: deepeval must not phone home from the worker.

deepeval 3.x starts PostHog and Sentry and looks up the public IP address
(api.ipify.org) while it is imported unless DEEPEVAL_TELEMETRY_OPT_OUT is set,
and uploads metric results (with the evaluated inputs and outputs) and traces
to Confident AI when it finds a Confident API key. The worker's evaluation
package sets the opt-outs before any of its modules imports deepeval, whatever
the process environment says.

The checks run in a fresh interpreter: deepeval reads the settings once, at
import, and the test process may have imported (or mocked) it already.
"""

from __future__ import annotations

import ast
import importlib.util
import json
import os
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

WORKERS_DIR = Path(__file__).resolve().parents[2]
REPO_ROOT = WORKERS_DIR.parent
CODEFORGE_DIR = WORKERS_DIR / "codeforge"
EVALUATION_DIR = CODEFORGE_DIR / "evaluation"

# The settings deepeval 3.8 honours, with the values that switch the traffic off.
PRIVACY_ENV = {
    "DEEPEVAL_TELEMETRY_OPT_OUT": "YES",  # PostHog, Sentry, public-IP lookup
    "DEEPEVAL_UPDATE_WARNING_OPT_IN": "0",  # PyPI version check
    "CONFIDENT_METRIC_LOGGING_ENABLED": "NO",  # metric uploads to Confident AI
    "CONFIDENT_TRACING_ENABLED": "NO",  # trace uploads to Confident AI
    "DEEPEVAL_DISABLE_DOTENV": "1",  # no keys merged from .env files in the cwd
    "DEEPEVAL_DISABLE_LEGACY_KEYFILE": "1",  # no Confident key from .deepeval/.deepeval
}

# Stops the interpreter the moment deepeval is about to be imported and prints
# the settings it would read, so deepeval itself never runs.
_ENV_AT_DEEPEVAL_IMPORT = textwrap.dedent(
    """
    import importlib.abc, json, os, sys

    class StopAtDeepeval(importlib.abc.MetaPathFinder):
        def find_spec(self, name, path=None, target=None):
            if name.split(".")[0] == "deepeval":
                sys.stdout.write(json.dumps({k: os.environ.get(k) for k in json.loads(sys.argv[2])}))
                sys.stdout.flush()
                os._exit(0)
            return None

    sys.meta_path.insert(0, StopAtDeepeval())
    __import__(sys.argv[1])
    sys.exit("deepeval was never imported")
    """
)


def _child_env(**overrides: str) -> dict[str, str]:
    """The current environment without any deepeval / Confident AI setting."""
    env = {
        key: value
        for key, value in os.environ.items()
        if not key.startswith(("DEEPEVAL_", "CONFIDENT_")) and key != "API_KEY"
    }
    env["PYTHONPATH"] = str(WORKERS_DIR)
    env.update(overrides)
    return env


def _run(args: list[str], env: dict[str, str], cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 - fixed interpreter and arguments
        [sys.executable, *args],
        env=env,
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=180,
        check=False,
    )


def _env_seen_by_deepeval(module: str, env: dict[str, str], cwd: Path) -> dict[str, str | None]:
    result = _run(["-c", _ENV_AT_DEEPEVAL_IMPORT, module, json.dumps(list(PRIVACY_ENV))], env, cwd)
    assert result.returncode == 0, result.stderr
    return json.loads(result.stdout)


@pytest.mark.parametrize("module", ["codeforge.evaluation.metrics", "codeforge.evaluation.litellm_judge"])
def test_opt_outs_are_set_before_deepeval_is_imported(module: str, tmp_path: Path) -> None:
    assert _env_seen_by_deepeval(module, _child_env(), tmp_path) == PRIVACY_ENV


def test_opt_outs_override_an_opt_in(tmp_path: Path) -> None:
    env = _child_env(
        DEEPEVAL_TELEMETRY_OPT_OUT="NO",
        DEEPEVAL_UPDATE_WARNING_OPT_IN="1",
        CONFIDENT_METRIC_LOGGING_ENABLED="YES",
        CONFIDENT_TRACING_ENABLED="YES",
    )
    assert _env_seen_by_deepeval("codeforge.evaluation.metrics", env, tmp_path) == PRIVACY_ENV


def _deepeval_imports(path: Path) -> list[int]:
    lines = []
    for node in ast.walk(ast.parse(path.read_text(), filename=str(path))):
        if isinstance(node, ast.Import):
            names = [alias.name for alias in node.names]
        elif isinstance(node, ast.ImportFrom):
            names = [node.module or ""]
        else:
            continue
        if any(name.split(".")[0] == "deepeval" for name in names):
            lines.append(node.lineno)
    return lines


def test_only_the_evaluation_package_imports_deepeval() -> None:
    """Its __init__ sets the opt-outs, which covers every module below it."""
    offenders = [
        f"{path.relative_to(WORKERS_DIR)}:{line}"
        for path in sorted(CODEFORGE_DIR.rglob("*.py"))
        for line in _deepeval_imports(path)
        if EVALUATION_DIR not in path.parents or path.name == "__init__.py"
    ]
    assert offenders == [], f"deepeval imported outside codeforge.evaluation submodules: {offenders}"


def test_worker_image_sets_the_opt_outs() -> None:
    """The image environment covers deepeval started outside the package (e.g. its CLI)."""
    dockerfile = (REPO_ROOT / "Dockerfile.worker").read_text()
    missing = [f"{key}={value}" for key, value in PRIVACY_ENV.items() if f"{key}={value}" not in dockerfile]
    assert missing == [], f"Dockerfile.worker lacks ENV {missing}"


_IMPORT_WITHOUT_NETWORK = textwrap.dedent(
    """
    import json, socket, sys

    attempts = []

    def refuse(self, address):
        attempts.append(repr(address))
        raise OSError("network disabled by the test")

    socket.socket.connect = refuse
    import codeforge.evaluation.metrics
    from deepeval.metrics.api import metric_data_manager
    from deepeval.telemetry import telemetry_opt_out
    from deepeval.tracing.utils import tracing_enabled

    sys.stdout.write(json.dumps({
        "attempts": attempts,
        "telemetry_opt_out": telemetry_opt_out(),
        "metric_logging_enabled": metric_data_manager.metric_logging_enabled,
        "tracing_enabled": tracing_enabled(),
    }))
    """
)


@pytest.mark.skipif(importlib.util.find_spec("deepeval") is None, reason="deepeval is not installed")
def test_real_deepeval_import_opens_no_connection(tmp_path: Path) -> None:
    result = _run(["-c", _IMPORT_WITHOUT_NETWORK], _child_env(), tmp_path)
    assert result.returncode == 0, result.stderr
    seen = json.loads(result.stdout)
    assert seen == {
        "attempts": [],
        "telemetry_opt_out": True,
        "metric_logging_enabled": False,
        "tracing_enabled": False,
    }
    assert not (tmp_path / ".deepeval").exists(), "deepeval created its telemetry directory"
