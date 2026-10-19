"""Which LLM providers have an API key (KI-125).

LiteLLM holds the provider keys and reports nothing about them. The worker
and the Go Core decide alike: a provider has a key when it is named in
``litellm.keyed_providers`` / ``CODEFORGE_LITELLM_KEYED_PROVIDERS`` (names
only: docker-compose.prod.yml derives them from the key variables without
passing a key) or when its key variable is set in the process environment.
Local servers need no key; a provider not listed here is assumed to have one.

The map equals ``providerKeyVars`` in internal/port/llm/provider_keys.go
(tests/test_provider_keys.py compares them). No imports from codeforge: the
settings module uses it.
"""

from __future__ import annotations

# Provider prefix of a model name -> the variable holding its API key in LiteLLM's environment.
PROVIDER_KEY_MAP: dict[str, str] = {
    "openai": "OPENAI_API_KEY",
    "anthropic": "ANTHROPIC_API_KEY",
    "gemini": "GEMINI_API_KEY",
    "groq": "GROQ_API_KEY",
    "mistral": "MISTRAL_API_KEY",
    "openrouter": "OPENROUTER_API_KEY",
    "cerebras": "CEREBRAS_API_KEY",
    "chutes": "CHUTES_API_KEY",
    "aihubmix": "AIHUBMIX_API_KEY",
    "deepseek": "DEEPSEEK_API_KEY",
    "cohere": "COHERE_API_KEY",
    "together_ai": "TOGETHERAI_API_KEY",
    "fireworks_ai": "FIREWORKS_API_KEY",
    "github_copilot": "GITHUB_TOKEN",
}

# Providers that never need an API key (local model servers).
KEYLESS_PROVIDERS: frozenset[str] = frozenset({"ollama", "lm_studio"})

KEYED_PROVIDERS_ENV = "CODEFORGE_LITELLM_KEYED_PROVIDERS"
_SETTING = f"{KEYED_PROVIDERS_ENV} / litellm.keyed_providers"


def parse_keyed_providers(env_value: str, yaml_value: object) -> frozenset[str]:
    """Resolve the named keyed providers: env var (comma-separated) > YAML list > none.

    Names are trimmed and lower-cased; local servers are accepted and ignored.
    A YAML value that is not a list (also when the env var wins), or a name
    not in PROVIDER_KEY_MAP, raises ValueError: a misspelt name would hide
    the provider's models silently.
    """
    if yaml_value is not None and not isinstance(yaml_value, list):
        msg = f"{_SETTING}: expected a list of provider names, got {type(yaml_value).__name__}"
        raise ValueError(msg)
    names = env_value.split(",") if env_value.strip() else [str(name) for name in yaml_value or []]

    keyed: set[str] = set()
    unknown: list[str] = []
    for raw in names:
        name = raw.strip().lower()
        if not name or name in KEYLESS_PROVIDERS:
            continue
        if name in PROVIDER_KEY_MAP:
            keyed.add(name)
        else:
            unknown.append(name)
    if unknown:
        msg = f"{_SETTING}: unknown provider(s) {', '.join(unknown)} (known: {', '.join(sorted(PROVIDER_KEY_MAP))})"
        raise ValueError(msg)
    return frozenset(keyed)


def model_provider(model: str) -> str:
    """Return the provider of a LiteLLM model name: its prefix ("openai/gpt-4o" -> "openai").

    "" for a name without a provider prefix. The Go Core resolves a user's
    own key by the same prefix (ResolveProviderAPIKey), and the key filter
    groups models by it.
    """
    provider, sep, _ = model.partition("/")
    return provider if sep else ""


def fallbacks_for_key(model: str, fallbacks: list[str]) -> list[str]:
    """Return the fallbacks a run with a user's own key may switch to.

    The key belongs to the provider of *model*, the model the Go Core resolved
    it for; it must never be sent to another provider. So only the fallbacks
    of that provider remain, none when *model* has no provider prefix.
    """
    provider = model_provider(model)
    return [m for m in fallbacks if provider and model_provider(m) == provider]
