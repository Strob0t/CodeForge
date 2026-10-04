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
    scrub_url_secrets,
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
    policy = OutboundPolicy(
        policy_case["allowed_private_hosts"],  # type: ignore[arg-type]
        trusted=bool(policy_case.get("trusted")),
    )
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


async def test_transport_dns_rebinding_never_reaches_the_new_address() -> None:
    """Within the cache ttl the request goes to the checked address; after it, the new one is checked."""
    answers = iter([["93.184.216.34"], ["10.0.0.7"]])

    async def rebinding(_host: str, _port: int) -> list[str]:
        return next(answers)

    now = [0.0]
    recorder = _Recorder()
    transport = GuardedTransport(
        OutboundPolicy([], resolver=rebinding), inner=httpx.MockTransport(recorder), clock=lambda: now[0]
    )
    async with httpx.AsyncClient(transport=transport) as client:
        assert (await client.get("http://rebind.example/a")).status_code == 204
        assert (await client.get("http://rebind.example/b")).status_code == 204
        now[0] += 31
        with pytest.raises(AddressRefusedError, match=r"10\.0\.0\.7"):
            await client.get("http://rebind.example/c")
    assert [str(r.url) for r in recorder.requests] == ["http://93.184.216.34/a", "http://93.184.216.34/b"]


async def test_transport_tries_the_next_checked_address() -> None:
    """Like Go's DialContext: every address is checked first, then they are tried in order (KI-100 review)."""

    def first_refuses(request: httpx.Request) -> httpx.Response:
        if request.url.host == "203.0.113.1":
            raise httpx.ConnectError("connection refused", request=request)
        return httpx.Response(204)

    recorder = _Recorder(first_refuses)
    policy = OutboundPolicy([], resolver=_resolver({"multi.example": ["203.0.113.1", "203.0.113.2"]}))
    async with _client(policy, recorder) as client:
        assert (await client.get("http://multi.example/a")).status_code == 204
    assert [r.url.host for r in recorder.requests] == ["203.0.113.1", "203.0.113.2"]

    # One refused address refuses the name before anything connects.
    recorder = _Recorder(first_refuses)
    policy = OutboundPolicy([], resolver=_resolver({"multi.example": ["203.0.113.1", "10.0.0.1"]}))
    async with _client(policy, recorder) as client:
        with pytest.raises(AddressRefusedError):
            await client.get("http://multi.example/a")
    assert recorder.requests == []

    # When every address fails, the last error is raised.
    recorder = _Recorder(lambda request: (_ for _ in ()).throw(httpx.ConnectError("down", request=request)))
    policy = OutboundPolicy([], resolver=_resolver({"multi.example": ["203.0.113.1", "203.0.113.2"]}))
    async with _client(policy, recorder) as client:
        with pytest.raises(httpx.ConnectError, match="down"):
            await client.get("http://multi.example/a")
    assert len(recorder.requests) == 2


def _slow_resolver(seconds: float):  # type: ignore[no-untyped-def]
    async def resolve(_host: str, _port: int) -> list[str]:
        await asyncio.sleep(seconds)
        return ["93.184.216.34"]

    return resolve


async def test_a_lookup_is_bounded_by_the_connect_timeout() -> None:
    recorder = _Recorder()
    policy = OutboundPolicy([], resolver=_slow_resolver(30))
    loop = asyncio.get_running_loop()
    started = loop.time()
    async with _client(policy, recorder, timeout=httpx.Timeout(5, connect=0.1)) as client:
        with pytest.raises(httpx.ConnectTimeout, match=r"DNS lookup of slow\.example timed out"):
            await client.get("http://slow.example/")
    assert loop.time() - started < 5
    assert recorder.requests == []

    with pytest.raises(httpx.ConnectTimeout, match=r"DNS lookup of slow\.example timed out"):
        await policy.check_url("http://slow.example/sse", timeout=0.1)


async def test_checked_addresses_are_cached_per_transport_and_host() -> None:
    calls: list[str] = []
    now = [100.0]
    policy = OutboundPolicy(
        [], resolver=_resolver({"a.example": ["93.184.216.34"], "b.example": ["93.184.216.35"]}, calls)
    )
    recorder = _Recorder()
    transport = GuardedTransport(policy, inner=httpx.MockTransport(recorder), clock=lambda: now[0])
    async with httpx.AsyncClient(transport=transport) as client:
        for _ in range(3):
            await client.get("http://a.example/")
        await client.get("http://b.example/")
        assert calls == ["a.example", "b.example"]
        now[0] += 29.9
        await client.get("http://a.example/")
        assert calls == ["a.example", "b.example"]
        now[0] += 0.2  # past the 30 s ttl of the first lookup
        await client.get("http://a.example/")
        assert calls == ["a.example", "b.example", "a.example"]
    assert [str(r.url) for r in recorder.requests][-1] == "http://93.184.216.34/"


async def test_a_refused_lookup_is_not_cached() -> None:
    calls: list[str] = []
    policy = OutboundPolicy([], resolver=_resolver({"internal.example": ["10.0.0.1"]}, calls))
    async with _client(policy, _Recorder()) as client:
        for _ in range(2):
            with pytest.raises(AddressRefusedError):
                await client.get("http://internal.example/")
    assert calls == ["internal.example", "internal.example"]


async def test_transport_falls_back_over_real_sockets(monkeypatch: pytest.MonkeyPatch) -> None:
    """127.0.0.2 refuses the connection (nothing listens there); 127.0.0.1 answers."""
    heads: list[bytes] = []

    async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        heads.append(await reader.readuntil(b"\r\n\r\n"))
        writer.write(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
        await writer.drain()
        writer.close()

    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    try:
        policy = OutboundPolicy(["127.0.0.0/8"], resolver=_resolver({"dev.example": ["127.0.0.2", "127.0.0.1"]}))
        async with guarded_client_factory(policy)() as client:
            response = await client.get(f"http://dev.example:{port}/mcp")
        assert response.status_code == 204
    finally:
        server.close()
        await server.wait_closed()
    assert len(heads) == 1


async def test_transport_refuses_a_redirect_to_a_private_address() -> None:
    def redirect(request: httpx.Request) -> httpx.Response:
        return httpx.Response(302, headers={"Location": "http://10.0.0.1/secret"})

    recorder = _Recorder(redirect)
    policy = OutboundPolicy([], resolver=_resolver({"public.example": ["93.184.216.34"]}))
    async with _client(policy, recorder, follow_redirects=True) as client:
        with pytest.raises(AddressRefusedError):
            await client.get("http://public.example/")
    assert len(recorder.requests) == 1


async def test_use_proxy_sends_requests_through_the_proxy_of_the_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    """mcp.use_proxy (KI-100 review): the proxy dials, so the address is not pinned."""
    request_lines: list[bytes] = []

    async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        head = await reader.readuntil(b"\r\n\r\n")
        request_lines.append(head.split(b"\r\n", 1)[0])
        writer.write(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
        await writer.drain()
        writer.close()

    proxy = await asyncio.start_server(handle, "127.0.0.1", 0)
    proxy_url = f"http://127.0.0.1:{proxy.sockets[0].getsockname()[1]}"
    for name in ["HTTP_PROXY", "http_proxy"]:
        monkeypatch.setenv(name, proxy_url)
    for name in ["HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"]:
        monkeypatch.delenv(name, raising=False)

    def unexpected(_host: str, _port: int) -> list[str]:
        raise AssertionError("the transport resolved the host although the proxy connects")

    try:
        factory = guarded_client_factory(OutboundPolicy([], resolver=unexpected), use_proxy=True)  # type: ignore[arg-type]
        async with factory(headers={"X-Api-Key": "k"}) as client:
            assert client.trust_env
            assert not isinstance(client._transport, GuardedTransport)
            response = await client.get("http://mcp.example.com/sse")
        assert response.status_code == 204
    finally:
        proxy.close()
        await proxy.wait_closed()
    assert request_lines == [b"GET http://mcp.example.com/sse HTTP/1.1"]


async def test_use_proxy_still_checks_the_url_before_connecting(monkeypatch: pytest.MonkeyPatch) -> None:
    def unexpected(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("the SDK client was opened for a refused url")

    monkeypatch.setattr("codeforge.mcp_workbench.sse_client", unexpected)
    conn = McpServerConnection(
        MCPServerDef(id="s1", name="S1", transport="sse", url="http://10.0.0.5:6280/sse", use_proxy=True)
    )
    with pytest.raises(AddressRefusedError, match=r"10\.0\.0\.5 is a private address"):
        await conn.connect()


async def test_connection_passes_use_proxy_to_the_client(monkeypatch: pytest.MonkeyPatch) -> None:
    from contextlib import asynccontextmanager

    used: dict[str, httpx.AsyncClient] = {}

    @asynccontextmanager
    async def sse_client(url: str, headers: object = None, httpx_client_factory: object = None, **_: object):  # type: ignore[no-untyped-def]
        used["client"] = httpx_client_factory()  # type: ignore[operator]
        yield (object(), object())

    monkeypatch.setattr("codeforge.mcp_workbench.sse_client", sse_client)
    monkeypatch.setattr("codeforge.mcp_workbench.ClientSession", _Session)
    monkeypatch.setattr(OutboundPolicy, "_lookup", _public_lookup)
    for use_proxy in [True, False]:
        conn = McpServerConnection(
            MCPServerDef(id="s1", name="S1", transport="sse", url="https://mcp.example.com/sse", use_proxy=use_proxy)
        )
        await conn.connect()
        client = used["client"]
        assert client.trust_env is use_proxy
        assert isinstance(client._transport, GuardedTransport) is not use_proxy
        await client.aclose()
        await conn.disconnect()


async def _public_lookup(_self: OutboundPolicy, _host: str, _port: int) -> list[str]:
    return ["93.184.216.34"]


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
                # A broad prefix does not open loopback; only an explicit entry does.
                allowed_private_hosts=["0.0.0.0/0", "::/0"],
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


@pytest.mark.parametrize(
    ("allowed", "trusted"),
    [(["127.0.0.1"], False), (["localhost"], False), (["127.0.0.0/8"], False), ([], True)],
    ids=["allowlisted address", "allowlisted name", "allowlisted prefix", "operator server"],
)
async def test_loopback_is_opened_by_an_explicit_entry_or_for_an_operator_server(
    allowed: list[str], trusted: bool, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The dev topology: docs-mcp published on 127.0.0.1:6280 (KI-100 review)."""
    from contextlib import asynccontextmanager

    used: dict[str, object] = {}

    @asynccontextmanager
    async def sse_client(url: str, headers: object = None, httpx_client_factory: object = None, **_: object):  # type: ignore[no-untyped-def]
        used["url"] = url
        yield (object(), object())

    async def lookup(_self: OutboundPolicy, _host: str, _port: int) -> list[str]:
        return ["127.0.0.1"]

    monkeypatch.setattr("codeforge.mcp_workbench.sse_client", sse_client)
    monkeypatch.setattr("codeforge.mcp_workbench.ClientSession", _Session)
    monkeypatch.setattr(OutboundPolicy, "_lookup", lookup)
    conn = McpServerConnection(
        MCPServerDef(
            id="s1",
            name="docs",
            transport="sse",
            url="http://localhost:6280/sse",
            allowed_private_hosts=allowed,
            trusted=trusted,
        )
    )
    await conn.connect()
    assert used["url"] == "http://localhost:6280/sse"
    await conn.disconnect()


async def test_an_operator_server_still_never_reaches_metadata() -> None:
    conn = McpServerConnection(
        MCPServerDef(id="s1", name="docs", transport="sse", url="http://169.254.169.254/latest/", trusted=True)
    )
    with pytest.raises(AddressRefusedError, match="link-local address; MCP servers may never use it"):
        await conn.connect()


@pytest.mark.parametrize("transport", ["sse", "streamable_http"])
async def test_a_failed_connect_logs_no_url_secrets(
    transport: str, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
) -> None:
    """KI-97 security review: httpx errors quote the request url with its userinfo and query."""
    token, key = "ghp_tokenvalue123", "sk-SECRETVALUE456"

    async def handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        await reader.readuntil(b"\r\n\r\n")
        writer.write(b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
        await writer.drain()
        writer.close()

    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]

    async def lookup(_self: OutboundPolicy, _host: str, _port: int) -> list[str]:
        return ["127.0.0.1"]

    monkeypatch.setattr(OutboundPolicy, "_lookup", lookup)
    try:
        with caplog.at_level(logging.DEBUG):
            wb = McpWorkbench()
            await wb.connect_servers(
                [
                    MCPServerDef(
                        id="s1",
                        name="S1",
                        transport=transport,
                        url=f"http://{token}@mcp.test:{port}/mcp?api_key={key}&x=1",
                        allowed_private_hosts=["127.0.0.1"],
                    )
                ]
            )
    finally:
        server.close()
        await server.wait_closed()
    assert wb._connections == {}
    logged = "\n".join(
        record.getMessage() + (logging.Formatter().formatException(record.exc_info) if record.exc_info else "")
        for record in caplog.records
        if record.name.startswith("codeforge")
    )
    assert "failed to connect to MCP server s1" in logged
    assert "mcp.test" in logged
    assert token not in logged
    assert key not in logged


def test_scrub_url_secrets() -> None:
    url = "http://svc:p%40ssw0rd@mcp.example/mcp?api_key=sk-a%2Bb123&region=europe#token=fragvalue"
    text = (
        f"Client error '401' for url '{url}'; retried http://svc:p%40ssw0rd@mcp.example/other "
        "with p@ssw0rd and sk-a+b123 (europe, fragvalue)"
    )
    scrubbed = scrub_url_secrets(text, url)
    for secret in ["p%40ssw0rd", "p@ssw0rd", "sk-a%2Bb123", "sk-a+b123", "europe", "fragvalue"]:
        assert secret not in scrubbed
    assert "mcp.example" in scrubbed
    assert scrub_url_secrets("ghp_tokenvalue123 failed", "https://ghp_tokenvalue123@h/") == "*** failed"
    assert "http://[bad" not in scrub_url_secrets("x http://[bad", "http://[bad")  # never raises


def test_server_def_reads_the_allowlist() -> None:
    default = MCPServerDef.model_validate({"id": "s", "name": "s", "transport": "sse", "url": "http://x"})
    assert default.allowed_private_hosts == []
    assert default.trusted is False
    sent = MCPServerDef.model_validate(
        {
            "id": "s",
            "name": "s",
            "transport": "sse",
            "url": "http://x",
            "allowed_private_hosts": ["docs-mcp"],
            "trusted": True,
        }
    )
    assert sent.allowed_private_hosts == ["docs-mcp"]
    assert sent.trusted is True


@pytest.mark.parametrize("fixture", ["runs_start.json", "conversation_run_start.json"])
def test_go_payloads_carry_the_allowlist(fixture: str) -> None:
    raw = json.loads((_CONTRACTS / fixture).read_text())
    server = MCPServerDef.model_validate(raw["mcp_servers"][0])
    assert server.allowed_private_hosts == ["docs-mcp", "10.20.0.0/16"]
    assert server.trusted is True
    assert server.use_proxy is True
