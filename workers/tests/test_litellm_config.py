"""The shipped LiteLLM config and the compose files that run it (KI-125, KI-130)."""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

from codeforge.llm import SCENARIO_DEFAULTS

ROOT = Path(__file__).resolve().parents[2]
CONFIG = yaml.safe_load((ROOT / "litellm" / "config.yaml").read_text())
ROUTES = {route["model_name"]: route for route in CONFIG["model_list"]}
# docker-compose.prod.yml exports these from its secret files in the entrypoint.
FROM_SECRETS = {"LITELLM_MASTER_KEY", "DATABASE_URL"}


def _litellm_environment(compose_file: str) -> set[str]:
    compose = yaml.safe_load((ROOT / compose_file).read_text())
    return set(compose["services"]["litellm"]["environment"])


def test_ollama_goes_to_its_openai_compatible_endpoint() -> None:
    """ollama/* (/api/generate) streamed tool calls as text; the /v1 endpoint sends tool_calls."""
    params = ROUTES["ollama/*"]["litellm_params"]

    assert params["model"] == "openai/*"
    assert params["api_base"] == "os.environ/OLLAMA_OPENAI_API_BASE"
    assert params["api_key"]


# The tag of a request without a scenario plus every scenario's tag.
ROUTE_TAGS = {"default"} | {scenario.tag for scenario in SCENARIO_DEFAULTS.values()}


def test_every_route_accepts_every_scenario_tag() -> None:
    """With tag filtering LiteLLM refuses a request whose tag its route lacks (401, KI-131)."""
    assert ROUTES
    for entry in CONFIG["model_list"]:
        tags = entry["litellm_params"].get("tags", [])
        assert set(tags) >= ROUTE_TAGS, f"{entry['model_name']} lacks {sorted(ROUTE_TAGS - set(tags))}"


@pytest.mark.parametrize("compose_file", ["docker-compose.yml", "docker-compose.prod.yml"])
def test_every_variable_the_config_reads_is_passed(compose_file: str) -> None:
    variables = set(re.findall(r"os\.environ/(\w+)", (ROOT / "litellm" / "config.yaml").read_text()))
    passed = _litellm_environment(compose_file) | FROM_SECRETS

    assert variables - passed == set()


@pytest.mark.parametrize("compose_file", ["docker-compose.yml", "docker-compose.prod.yml"])
def test_ollama_variables_derive_from_one_url(compose_file: str) -> None:
    compose = yaml.safe_load((ROOT / compose_file).read_text())
    environment = compose["services"]["litellm"]["environment"]

    assert environment["OLLAMA_API_BASE"] == "${OLLAMA_BASE_URL:-http://host.docker.internal:11434}"
    assert environment["OLLAMA_OPENAI_API_BASE"] == environment["OLLAMA_API_BASE"] + "/v1"


def test_wildcard_routes_are_not_health_probed() -> None:
    """LiteLLM probes a wildcard route with a random catalogue model, which fails on local servers (KI-130)."""
    wildcards = [name for name in ROUTES if name.endswith("/*")]

    assert wildcards
    for name in wildcards:
        assert ROUTES[name].get("model_info", {}).get("disable_background_health_check") is True, name
    assert CONFIG["general_settings"]["health_check_skip_disabled_background_models"] is True
