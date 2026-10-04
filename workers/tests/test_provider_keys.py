"""Which providers have an API key: the same answer in the Go Core and the worker (KI-125 review).

LiteLLM holds the provider keys; the Core and the worker see only names
(litellm.keyed_providers / CODEFORGE_LITELLM_KEYED_PROVIDERS, derived by
docker-compose.prod.yml from the key variables) plus the key variables of
their own environment. The Core lists a keyed provider's models and picks its
default model from them; the worker's routing and default model use the same
rule, so neither picks a model of a provider without a key.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

from codeforge.config import get_settings, load_yaml_config
from codeforge.provider_keys import KEYED_PROVIDERS_ENV, KEYLESS_PROVIDERS, PROVIDER_KEY_MAP
from codeforge.routing.key_filter import filter_keyless_models, reset_warnings

REPO_ROOT = Path(__file__).resolve().parents[2]


@pytest.fixture(autouse=True)
def _no_keys(monkeypatch: pytest.MonkeyPatch) -> None:
    for env_var in PROVIDER_KEY_MAP.values():
        monkeypatch.delenv(env_var, raising=False)
    monkeypatch.delenv(KEYED_PROVIDERS_ENV, raising=False)
    reset_warnings()


def _go_map() -> dict[str, str]:
    source = (REPO_ROOT / "internal/port/llm/provider_keys.go").read_text()
    block = re.search(r"var providerKeyVars = map\[string\]string\{(.*?)\n\}", source, re.DOTALL)
    assert block, "providerKeyVars not found in provider_keys.go"
    return dict(re.findall(r'"(\w+)":\s*"(\w+)"', block.group(1)))


def _go_keyless() -> set[str]:
    source = (REPO_ROOT / "internal/port/llm/provider_keys.go").read_text()
    block = re.search(r"var keylessProviders = map\[string\]bool\{(.*?)\}", source, re.DOTALL)
    assert block, "keylessProviders not found in provider_keys.go"
    return set(re.findall(r'"(\w+)":\s*true', block.group(1)))


def test_go_and_worker_know_the_same_provider_keys() -> None:
    assert _go_map() == PROVIDER_KEY_MAP
    assert _go_keyless() == set(KEYLESS_PROVIDERS)


def test_every_shipped_cloud_route_has_a_key_variable() -> None:
    """A shipped route without an entry would count as keyed in both (unknown provider)."""
    import yaml

    config = yaml.safe_load((REPO_ROOT / "litellm/config.yaml").read_text())
    for entry in config["model_list"]:
        name = entry["model_name"]
        if not name.endswith("/*"):
            continue
        provider = name.split("/", 1)[0]
        if provider in KEYLESS_PROVIDERS:
            continue
        assert provider in PROVIDER_KEY_MAP, f"{name}: add its key variable to both provider maps"
        assert entry["litellm_params"]["api_key"] == f"os.environ/{PROVIDER_KEY_MAP[provider]}", name


def test_prod_compose_names_every_keyed_provider_without_its_key() -> None:
    import yaml

    compose = yaml.safe_load((REPO_ROOT / "docker-compose.prod.yml").read_text())
    for service in ("core", "worker"):
        value = compose["services"][service]["environment"][KEYED_PROVIDERS_ENV]
        # Only ${VAR:+name,}: the name when the key is set, never the key itself.
        assert re.fullmatch(r"(\$\{\w+:\+\w+,\})+", value), f"{service}: {value!r}"
        named = dict(re.findall(r"\$\{(\w+):\+(\w+),\}", value))
        for env_var, provider in named.items():
            assert PROVIDER_KEY_MAP[provider] == env_var, (service, provider)
        # Every provider key LiteLLM gets is named (GITHUB_TOKEN: the Copilot
        # route is disabled, the token serves other GitHub calls).
        litellm_keys = {
            var
            for var in compose["services"]["litellm"]["environment"]
            if var in PROVIDER_KEY_MAP.values() and var != "GITHUB_TOKEN"
        }
        assert set(named) == litellm_keys, service


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        pytest.param("", frozenset(), id="unset"),
        pytest.param("anthropic,groq,", frozenset({"anthropic", "groq"}), id="compose-trailing-comma"),
        pytest.param(" Anthropic , ,openai", frozenset({"anthropic", "openai"}), id="whitespace-and-case"),
        pytest.param("ollama", frozenset(), id="keyless-provider-is-accepted"),
    ],
)
def test_keyed_providers_from_env(value: str, expected: frozenset[str], monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv(KEYED_PROVIDERS_ENV, value)
    assert get_settings().keyed_providers == expected


@pytest.mark.parametrize("value", ["antropic", "openai,gpt"])
def test_unknown_keyed_provider_stops_startup(value: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv(KEYED_PROVIDERS_ENV, value)
    with pytest.raises(ValueError, match="keyed_providers"):
        get_settings()


def _yaml(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, text: str) -> None:
    config = tmp_path / "codeforge.yaml"
    config.write_text(text)
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    load_yaml_config.cache_clear()


def test_keyed_providers_from_yaml_env_wins(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _yaml(tmp_path, monkeypatch, "litellm:\n  keyed_providers:\n    - anthropic\n")
    try:
        assert get_settings().keyed_providers == frozenset({"anthropic"})
        get_settings.cache_clear()
        monkeypatch.setenv(KEYED_PROVIDERS_ENV, "openai")
        assert get_settings().keyed_providers == frozenset({"openai"})
    finally:
        load_yaml_config.cache_clear()


@pytest.mark.parametrize("text", ["litellm:\n  keyed_providers: anthropic\n", "litellm:\n  keyed_providers: {a: 1}\n"])
def test_wrongly_typed_yaml_keyed_providers_stops_startup(
    text: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _yaml(tmp_path, monkeypatch, text)
    try:
        with pytest.raises(ValueError, match=r"litellm\.keyed_providers"):
            get_settings()
    finally:
        load_yaml_config.cache_clear()


class TestKeyFilter:
    def test_named_provider_is_kept_without_its_key_in_the_environment(self, monkeypatch: pytest.MonkeyPatch) -> None:
        # The production worker has no provider key in its environment.
        monkeypatch.setenv(KEYED_PROVIDERS_ENV, "anthropic")
        assert filter_keyless_models(["anthropic/claude-sonnet-4-5", "openai/gpt-4o"]) == [
            "anthropic/claude-sonnet-4-5"
        ]

    @pytest.mark.parametrize("model", ["openrouter/meta-llama/llama-3.3-70b", "cerebras/llama3.1-8b", "chutes/x/y"])
    def test_catalogue_routes_without_a_key_are_dropped(self, model: str) -> None:
        # The Core lists them as one route row; the router must not pick one either.
        assert filter_keyless_models([model]) == []

    def test_key_variable_still_counts(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("OPENROUTER_API_KEY", "sk-or-1")
        assert filter_keyless_models(["openrouter/x/y"]) == ["openrouter/x/y"]
