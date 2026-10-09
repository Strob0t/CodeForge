"""The tenant of the NATS message being handled (KI-64).

The Go Core stamps the tenant on every message it publishes (header
``X-Tenant-ID``). The consumer loop binds it while the message is handled,
and the worker's JetStream context adds it to everything published meanwhile,
also from tasks the handler starts (they inherit the context). A result or
event therefore reaches the right tenant even when its payload carries no
``tenant_id``.
"""

from __future__ import annotations

from contextvars import ContextVar, Token

from codeforge.nats_subjects import HEADER_TENANT_ID

_current_tenant: ContextVar[str] = ContextVar("codeforge_tenant", default="")


def current_tenant() -> str:
    """The tenant of the message being handled, or ""."""
    return _current_tenant.get()


def bind_tenant(tenant_id: str) -> Token[str]:
    """Make *tenant_id* the current tenant; pass the token to reset_tenant when done."""
    return _current_tenant.set(tenant_id)


def reset_tenant(token: Token[str]) -> None:
    """Restore the tenant that was current before bind_tenant."""
    _current_tenant.reset(token)


def tenant_of(headers: dict[str, str] | None) -> str:
    """The tenant a message's headers carry, or ""."""
    return (headers or {}).get(HEADER_TENANT_ID, "")


def with_tenant_header(headers: dict[str, str] | None) -> dict[str, str] | None:
    """Add the current tenant to outgoing headers unless they already name one."""
    tenant = current_tenant()
    if not tenant or (headers and HEADER_TENANT_ID in headers):
        return headers
    return {**(headers or {}), HEADER_TENANT_ID: tenant}
