"""SSRF protection of the worker's MCP connections (KI-100).

A tenant admin chooses the url of an sse or streamable_http MCP server, and the
worker connects to it in runs. It never reaches loopback, link-local (cloud
metadata), unspecified, multicast or reserved addresses, and reaches private
ones only for the hosts the platform operator allowlisted (Go sends the list
as ``allowed_private_hosts`` with each server). The address a request connects
to is the address that was checked.
"""

from __future__ import annotations

import asyncio
import json
import logging
from pathlib import Path

import httpx
import pytest

from codeforge.mcp_models import MCPServerDef
from codeforge.mcp_outbound import (
    AddressRefusedError,
    GuardedTransport,
    OutboundPolicy,
    guarded_client_factory,
)
from codeforge.mcp_workbench import McpServerConnection, McpWorkbench

_CONTRACTS = Path(__file__).parents[2] / "internal" / "port" / "messagequeue" / "testdata" / "contracts"
# Shared with internal/netutil/outbound_test.go: the worker and the Go Core decide alike.
_CASES = json.loads(
    (Path(__file__).parents[2] / "internal" / "netutil" / "testdata" / "outbound_cases.json").read_text()
)


def _policy_cases() -> list[object]:
    return [
        pytest.param(policy, case, id=f"{policy['name']}/{case['name']}")
        for policy in _CASES["policies"]
        for case in policy["cases"]
    ]


@pytest.mark.parametrize(("policy_case", "case"), _policy_cases())
def test_check_address(policy_case: dict[str, object], case: dict[str, object]) -> None:
    policy = OutboundPolicy(policy_case["allowed_private_hosts"])  # type: ignore[arg-type]
    host, ip = str(case["host"]), str(case["ip"])
    if case.get("allowed"):
        policy.check_address(host, ip)
        return
    with pytest.raises(AddressRefusedError) as refused:
        policy.check_address(host, ip)
    assert refused.value.allowable is bool(case.get("allowable"))


@pytest.mark.parametrize("entry", _CASES["invalid_entries"])
def test_invalid_allowlist_entries_allow_nothing(entry: str, caplog: pytest.LogCaptureFixture) -> None:
    """Go refuses them at startup; a payload carrying one anyway allows nothing (fail closed)."""
    with caplog.at_level(logging.WARNING):
        policy = OutboundPolicy([entry, "docs-mcp"])
    assert "ignoring invalid allowed private host" in caplog.text
    with pytest.raises(AddressRefusedError):
        policy.check_address("docs-mcp.evil", "10.0.0.1")
    policy.check_address("docs-mcp", "10.0.0.1")


def test_valid_allowlist_entries(caplog: pytest.LogCaptureFixture) -> None:
    with caplog.at_level(logging.WARNING):
        OutboundPolicy(_CASES["valid_entries"])
    assert caplog.text == ""


def test_messages_name_the_address() -> None:
    policy = OutboundPolicy([])
    with pytest.raises(AddressRefusedError, match=r"localhost resolves to 127\.0\.0\.1, a loopback address"):
        policy.check_address("localhost", "::ffff:127.0.0.1")
    with pytest.raises(AddressRefusedError, match=r"10\.0\.0\.5 is a private address"):
        policy.check_address("10.0.0.5", "10.0.0.5")
    with pytest.raises(AddressRefusedError, match=r":: is an unspecified address"):
        policy.check_address("::", "::")


def _resolver(names: dict[str, list[str]], calls: list[str] | None = None):  # type: ignore[no-untyped-def]
    async def resolve(host: str, _port: int) -> list[str]:
        if calls is not None:
            calls.append(host)
        if host not in names:
            raise OSError(f"no such host: {host}")
        return names[host]

    return resolve


async def test_resolve_checks_every_address() -> None:
    policy = OutboundPolicy(
        ["docs-mcp"],
        resolver=_resolver(
            {
                "public.example": ["93.184.216.34"],
                "internal.example": ["10.1.2.3"],
                "mixed.example": ["93.184.216.34", "127.0.0.1"],
                "docs-mcp": ["172.18.0.5"],
            }
        ),
    )
    assert await policy.resolve("public.example", 80) == ["93.184.216.34"]
    assert await policy.resolve("docs-mcp", 6280) == ["172.18.0.5"]
    assert await policy.resolve("93.184.216.34", 80) == ["93.184.216.34"]
    with pytest.raises(AddressRefusedError) as refused:
        await policy.resolve("internal.example", 80)
    assert refused.value.allowable
    with pytest.raises(AddressRefusedError):
        await policy.resolve("mixed.example", 80)
    with pytest.raises(AddressRefusedError):
        await policy.resolve("169.254.169.254", 80)


async def test_check_url_refuses_other_schemes_and_internal_hosts() -> None:
    policy = OutboundPolicy([], resolver=_resolver({"public.example": ["93.184.216.34"]}))
    await policy.check_url("https://public.example/mcp")
    for url in ["http://127.0.0.1:6280/sse", "http://[::1]/sse", "http://169.254.169.254/latest/meta-data/"]:
        with pytest.raises(AddressRefusedError):
            await policy.check_url(url)
    for url in ["file:///etc/passwd", "ftp://public.example/", "http:///nohost"]:
        with pytest.raises(AddressRefusedError, match="http or https"):
            await policy.check_url(url)


# --- The transport connects only to checked addresses ---


class _Recorder:
    """An inner transport that records the requests it is given."""

    def __init__(self, handler=None) -> None:  # type: ignore[no-untyped-def]
        self.requests: list[httpx.Request] = []
        self._handler = handler or (lambda _req: httpx.Response(204))

    async def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        return self._handler(request)


def _client(policy: OutboundPolicy, recorder: _Recorder, **kwargs: object) -> httpx.AsyncClient:
    transport = GuardedTransport(policy, inner=httpx.MockTransport(recorder))
    return httpx.AsyncClient(transport=transport, **kwargs)  # type: ignore[arg-type]


async def test_transport_pins_the_checked_address() -> None:
    policy = OutboundPolicy(
        ["docs-mcp"], resolver=_resolver({"docs-mcp": ["172.18.0.5"], "public.example": ["2606:2800::1"]})
    )
    recorder = _Recorder()
    async with _client(policy, recorder) as client:
        await client.post("http://docs-mcp:6280/mcp", json={"a": 1}, headers={"Authorization": "Bearer t"})
        await client.get("https://public.example/sse")

    sent, tls = recorder.requests
    assert sent.url == httpx.URL("http://172.18.0.5:6280/mcp")
    assert sent.headers["host"] == "docs-mcp:6280"
    assert sent.headers["authorization"] == "Bearer t"
    assert await sent.aread() == b'{"a":1}'
    assert sent.extensions["sni_hostname"] == "docs-mcp"
    assert tls.url == httpx.URL("https://[2606:2800::1]/sse")
    assert tls.headers["host"] == "public.example"
    assert tls.extensions["sni_hostname"] == "public.example"


async def test_transport_refuses_before_connecting() -> None:
    recorder = _Recorder()
    async with _client(OutboundPolicy([]), recorder) as client:
        for url in ["http://127.0.0.1:6280/", "http://[::1]/", "http://10.0.0.1/", "http://169.254.169.254/"]:
            with pytest.raises(AddressRefusedError):
                await client.get(url)
    assert recorder.requests == []


async def test_transport_checks_every_request_dns_rebinding() -> None:
    answers = iter([["93.184.216.34"], ["10.0.0.7"]])

    async def rebinding(_host: str, _port: int) -> list[str]:
        return next(answers)

    recorder = _Recorder()
    async with _client(OutboundPolicy([], resolver=rebinding), recorder) as client:
        assert (await client.get("http://rebind.example/a")).status_code == 204
        with pytest.raises(AddressRefusedError, match=r"10\.0\.0\.7"):
            await client.get("http://rebind.example/b")
    assert [str(r.url) for r in recorder.requests] == ["http://93.184.216.34/a"]


async def test_transport_refuses_a_redirect_to_a_private_address() -> None:
    def redirect(request: httpx.Request) -> httpx.Response:
        return httpx.Response(302, headers={"Location": "http://10.0.0.1/secret"})

    recorder = _Recorder(redirect)
    policy = OutboundPolicy([], resolver=_resolver({"public.example": ["93.184.216.34"]}))
    async with _client(policy, recorder, follow_redirects=True) as client:
        with pytest.raises(AddressRefusedError):
            await client.get("http://public.example/")
    assert len(recorder.requests) == 1


async def test_guarded_client_factory_uses_the_policy_and_no_proxy(monkeypatch: pytest.MonkeyPatch) -> None:
    # With a proxy, the proxy would choose the address the policy checks.
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.example:3128")
    monkeypatch.setenv("HTTPS_PROXY", "http://proxy.example:3128")
    monkeypatch.setenv("ALL_PROXY", "http://proxy.example:3128")
    factory = guarded_client_factory(OutboundPolicy([], resolver=_resolver({"internal.example": ["10.0.0.9"]})))
    async with factory(headers={"X-Api-Key": "k"}) as client:
        assert client.headers["x-api-key"] == "k"
        assert client.timeout.read == 300.0
        for url in ["http://internal.example/", "https://internal.example/"]:
            with pytest.raises(AddressRefusedError, match=r"internal\.example resolves to 10\.0\.0\.9"):
                await client.get(url)


async def test_a_pinned_request_reaches_the_server_with_the_host_header(monkeypatch: pytest.MonkeyPatch) -> None:
    """Over a real socket: the request goes to the checked address and still names the host.

    Loopback is refused, so the test lets it through as a private address and
    allowlists it, standing in for a server on the deployment network.
    """
    from codeforge import mcp_outbound

    monkeypatch.setattr(
        mcp_outbound, "_NEVER_ALLOWED", tuple(r for r in mcp_outbound._NEVER_ALLOWED if r[1] != "loopback")
    )
    monkeypatch.setattr(
        mcp_outbound,
        "_PRIVATE_NETWORKS",
        (*mcp_outbound._PRIVATE_NETWORKS, mcp_outbound.ipaddress.ip_network("127.0.0.0/8")),
    )
    heads: list[bytes] = []

    async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        heads.append(await reader.readuntil(b"\r\n\r\n"))
        writer.write(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
        await writer.drain()
        writer.close()

    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    try:
        policy = OutboundPolicy(["docs-mcp"], resolver=_resolver({"docs-mcp": ["127.0.0.1"]}))
        async with guarded_client_factory(policy)() as client:
            response = await client.get(f"http://docs-mcp:{port}/mcp")
        assert response.status_code == 204
    finally:
        server.close()
        await server.wait_closed()
    assert heads[0].startswith(b"GET /mcp HTTP/1.1\r\n")
    assert f"\r\nhost: docs-mcp:{port}\r\n".encode() in heads[0].lower()


# --- MCP connections ---


async def _local_server() -> tuple[asyncio.Server, list[str]]:
    """A TCP server on 127.0.0.1 that records connections."""
    seen: list[str] = []

    async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        seen.append("connection")
        writer.close()

    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    return server, seen


@pytest.mark.parametrize("transport", ["sse", "streamable_http"])
async def test_connection_refuses_a_loopback_url_before_connecting(transport: str) -> None:
    server, seen = await _local_server()
    port = server.sockets[0].getsockname()[1]
    try:
        conn = McpServerConnection(
            MCPServerDef(
                id="s1",
                name="S1",
                transport=transport,
                url=f"http://127.0.0.1:{port}/mcp",
                allowed_private_hosts=["127.0.0.1", "localhost"],
            )
        )
        with pytest.raises(AddressRefusedError, match=r"127\.0\.0\.1 is a loopback address"):
            await conn.connect()
        assert not conn.connected
    finally:
        server.close()
        await server.wait_closed()
    assert seen == []


async def test_workbench_skips_a_refused_server_with_the_reason(caplog: pytest.LogCaptureFixture) -> None:
    wb = McpWorkbench()
    with caplog.at_level(logging.ERROR):
        await wb.connect_servers([MCPServerDef(id="s1", name="S1", transport="sse", url="http://10.0.0.5:6280/sse")])
    assert wb._connections == {}
    assert "10.0.0.5 is a private address" in caplog.text


class _Session:
    """A ClientSession stand-in whose handshake succeeds."""

    def __init__(self, *_streams: object) -> None:
        pass

    async def __aenter__(self) -> _Session:
        return self

    async def __aexit__(self, *_exc: object) -> None:
        return None

    async def initialize(self) -> None:
        return None


async def _docs_mcp_lookup(_self: OutboundPolicy, _host: str, _port: int) -> list[str]:
    return ["172.18.0.5"]


async def test_sse_connection_uses_the_allowlist_and_the_guarded_client(monkeypatch: pytest.MonkeyPatch) -> None:
    from contextlib import asynccontextmanager

    used: dict[str, object] = {}

    @asynccontextmanager
    async def sse_client(url: str, headers: object = None, httpx_client_factory: object = None, **_: object):  # type: ignore[no-untyped-def]
        used["url"] = url
        used["client"] = httpx_client_factory()  # type: ignore[operator]
        yield (object(), object())

    monkeypatch.setattr("codeforge.mcp_workbench.sse_client", sse_client)
    monkeypatch.setattr("codeforge.mcp_workbench.ClientSession", _Session)
    monkeypatch.setattr(OutboundPolicy, "_lookup", _docs_mcp_lookup)
    conn = McpServerConnection(
        MCPServerDef(
            id="s1", name="S1", transport="sse", url="http://docs-mcp:6280/sse", allowed_private_hosts=["docs-mcp"]
        )
    )
    await conn.connect()
    assert conn.connected
    assert used["url"] == "http://docs-mcp:6280/sse"
    assert isinstance(used["client"]._transport, GuardedTransport)  # type: ignore[attr-defined]
    await conn.disconnect()


async def test_streamable_http_connection_unpacks_the_sdk_streams(monkeypatch: pytest.MonkeyPatch) -> None:
    """The SDK yields (read, write, get_session_id); the worker unpacked two values and never connected."""
    from contextlib import asynccontextmanager

    used: dict[str, object] = {}

    @asynccontextmanager
    async def streamable_http_client(url: str, *, http_client: httpx.AsyncClient, **_: object):  # type: ignore[no-untyped-def]
        used["url"] = url
        used["client"] = http_client
        yield (object(), object(), lambda: None)

    monkeypatch.setattr("codeforge.mcp_workbench.streamable_http_client", streamable_http_client)
    monkeypatch.setattr("codeforge.mcp_workbench.ClientSession", _Session)
    monkeypatch.setattr(OutboundPolicy, "_lookup", _docs_mcp_lookup)
    conn = McpServerConnection(
        MCPServerDef(
            id="s1",
            name="S1",
            transport="streamable_http",
            url="http://docs-mcp:6280/mcp",
            headers={"Authorization": "Bearer t"},
            allowed_private_hosts=["docs-mcp"],
        )
    )
    await conn.connect()
    assert conn.connected
    client = used["client"]
    assert isinstance(client, httpx.AsyncClient)
    assert isinstance(client._transport, GuardedTransport)
    assert client.headers["authorization"] == "Bearer t"
    await conn.disconnect()
    assert client.is_closed


async def test_a_private_host_without_the_allowlist_never_reaches_the_sdk(monkeypatch: pytest.MonkeyPatch) -> None:
    def unexpected(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("the SDK client was opened for a refused url")

    monkeypatch.setattr("codeforge.mcp_workbench.streamable_http_client", unexpected)
    monkeypatch.setattr("codeforge.mcp_workbench.sse_client", unexpected)
    monkeypatch.setattr(OutboundPolicy, "_lookup", _docs_mcp_lookup)
    for transport in ["sse", "streamable_http"]:
        conn = McpServerConnection(
            MCPServerDef(id="s1", name="S1", transport=transport, url="http://docs-mcp:6280/mcp")
        )
        with pytest.raises(AddressRefusedError, match=r"docs-mcp resolves to 172\.18\.0\.5, a private address"):
            await conn.connect()


def test_server_def_reads_the_allowlist() -> None:
    default = MCPServerDef.model_validate({"id": "s", "name": "s", "transport": "sse", "url": "http://x"})
    assert default.allowed_private_hosts == []
    sent = MCPServerDef.model_validate(
        {"id": "s", "name": "s", "transport": "sse", "url": "http://x", "allowed_private_hosts": ["docs-mcp"]}
    )
    assert sent.allowed_private_hosts == ["docs-mcp"]


@pytest.mark.parametrize("fixture", ["runs_start.json", "conversation_run_start.json"])
def test_go_payloads_carry_the_allowlist(fixture: str) -> None:
    raw = json.loads((_CONTRACTS / fixture).read_text())
    server = MCPServerDef.model_validate(raw["mcp_servers"][0])
    assert server.allowed_private_hosts == ["docs-mcp", "10.20.0.0/16"]
