"""SSRF protection for the worker's sse and streamable_http MCP connections (KI-100).

A tenant admin chooses the url of such a server, and the worker connects to it
in runs. Link-local (cloud metadata), unspecified, multicast and reserved
addresses are never used. Private ones (RFC 1918, ULA, CGNAT, ...) are used
only for the host names, addresses and CIDR prefixes the platform operator
allowlisted (``mcp.allowed_private_hosts``; Go sends the list with each server
as ``allowed_private_hosts``), loopback ones only for an explicit loopback entry
(``localhost``, ``127.0.0.1``, ``::1`` or a loopback prefix). An operator server
(``servers_dir``, sent with ``trusted``) may use private and loopback addresses.
The rules match ``internal/netutil/outbound.go``.

``GuardedTransport`` resolves the host of every request, checks every address
and sends the request to a checked address (the Host header and TLS server name
stay the host's), so DNS rebinding and redirects cannot reach a refused address.
"""

from __future__ import annotations

import asyncio
import ipaddress
import logging
import re
import socket
import time
from typing import TYPE_CHECKING

import httpx
from mcp.shared._httpx_utils import MCP_DEFAULT_SSE_READ_TIMEOUT, MCP_DEFAULT_TIMEOUT

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Sequence

    from mcp.shared._httpx_utils import McpHttpClientFactory

    Resolver = Callable[[str, int], Awaitable[list[str]]]

logger = logging.getLogger(__name__)

IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address
IPNetwork = ipaddress.IPv4Network | ipaddress.IPv6Network

_PRIVATE = "private"
_LOOPBACK = "loopback"

# Refused by default: loopback (only an explicit allowlist entry or an operator
# server opens it) and, for every server, the rest.
_NEVER_ALLOWED: tuple[tuple[IPNetwork, str], ...] = tuple(
    (ipaddress.ip_network(net), kind)
    for net, kind in (
        ("0.0.0.0/8", "unspecified"),
        ("127.0.0.0/8", "loopback"),
        ("169.254.0.0/16", "link-local"),
        ("100.100.100.200/32", "cloud metadata"),
        ("224.0.0.0/4", "multicast"),
        ("240.0.0.0/4", "reserved"),
        ("::/128", "unspecified"),
        ("::1/128", "loopback"),
        ("fe80::/10", "link-local"),
        ("fd00:ec2::254/128", "cloud metadata"),
        ("ff00::/8", "multicast"),
    )
)

# Refused unless the host or the address is allowlisted.
_PRIVATE_NETWORKS: tuple[IPNetwork, ...] = tuple(
    ipaddress.ip_network(net)
    for net in (
        "10.0.0.0/8",
        "172.16.0.0/12",
        "192.168.0.0/16",
        "100.64.0.0/10",
        "198.18.0.0/15",
        "192.0.0.0/24",
        "fc00::/7",
        "fec0::/10",
        # NAT64 local-use prefix (RFC 8215): its IPv4 embedding is not the /96 one, so not read as IPv4.
        "64:ff9b:1::/48",
    )
)

# IPv6 ranges that carry an IPv4 address in their last 32 bits: deprecated
# IPv4-compatible addresses and the NAT64 well-known prefix.
_EMBEDS_IPV4: tuple[IPNetwork, ...] = (ipaddress.ip_network("::/96"), ipaddress.ip_network("64:ff9b::/96"))

# The loopback ranges an explicit allowlist entry can open.
_LOOPBACK_NETWORKS: tuple[IPNetwork, ...] = (ipaddress.ip_network("127.0.0.0/8"), ipaddress.ip_network("::1/128"))

# How long a transport keeps the checked addresses of a host (seconds).
_ADDRESS_CACHE_TTL = 30.0

_HOST_LABEL = re.compile(r"^[A-Za-z0-9_-]{1,63}$")
_DEFAULT_PORTS = {"http": 80, "https": 443}


class AddressRefusedError(httpx.ConnectError):
    """A url or address the outbound policy refuses."""

    def __init__(self, message: str, *, allowable: bool = False) -> None:
        super().__init__(message)
        self.allowable = allowable
        """True when the operator could allow the address (private, or loopback with an explicit entry)."""


def _address(value: str | IPAddress) -> IPAddress:
    if isinstance(value, str):
        return ipaddress.ip_address(value.split("%", 1)[0])
    return value


def _classify(address: IPAddress) -> tuple[IPAddress, str]:
    """The address the kind was decided on (IPv4 inside IPv6 as IPv4) and its kind ("" when public)."""
    if isinstance(address, ipaddress.IPv6Address) and address.ipv4_mapped is not None:
        address = address.ipv4_mapped
    kind = _kind_of(address)
    if kind or isinstance(address, ipaddress.IPv4Address):
        return address, kind
    if any(address in net for net in _EMBEDS_IPV4):
        v4 = ipaddress.IPv4Address(int(address) & 0xFFFFFFFF)
        return v4, _kind_of(v4)
    return address, ""


def _kind_of(address: IPAddress) -> str:
    for net, kind in _NEVER_ALLOWED:
        if address in net:
            return kind
    if any(address in net for net in _PRIVATE_NETWORKS):
        return _PRIVATE
    return ""


def _unmap_network(network: IPNetwork) -> IPNetwork:
    """An IPv4-mapped IPv6 entry (::ffff:10.0.0.0/104) as the IPv4 network it maps, as Go does."""
    mapped = network.network_address.ipv4_mapped if isinstance(network, ipaddress.IPv6Network) else None
    if mapped is None or network.prefixlen < 96:
        return network
    return ipaddress.IPv4Network((mapped, network.prefixlen - 96))


def _normalise_host(host: str) -> str:
    return host.lower().removesuffix(".")


def _is_host_name(entry: str) -> bool:
    if not entry or len(entry) > 253:
        return False
    return all(_HOST_LABEL.match(label) for label in entry.removesuffix(".").split("."))


def _is_ip_literal(host: str) -> bool:
    try:
        _address(host)
    except ValueError:
        return False
    return True


class OutboundPolicy:
    """Decides which addresses an MCP connection may reach."""

    def __init__(
        self, allowed_private_hosts: Sequence[str], *, trusted: bool = False, resolver: Resolver | None = None
    ) -> None:
        self._hosts: set[str] = set()
        self._networks: list[IPNetwork] = []
        self._loopback: list[IPNetwork] = []
        self._trusted = trusted
        self._resolver = resolver
        for raw in allowed_private_hosts:
            entry = raw.strip()
            try:
                network = _unmap_network(ipaddress.ip_network(entry, strict=False))
            except ValueError:
                network = None
            if network is not None:
                self._networks.append(network)
                if any(network.subnet_of(lo) for lo in _LOOPBACK_NETWORKS if lo.version == network.version):
                    self._loopback.append(network)
                continue
            if _is_host_name(entry):
                self._hosts.add(_normalise_host(entry))
            else:
                # Go validates the list; an entry that is none of these allows nothing.
                logger.warning("ignoring invalid allowed private host %r", raw)

    def check_address(self, host: str, address: str | IPAddress) -> None:
        """Raise AddressRefusedError when a connection to host may not reach address."""
        original = _address(address)
        effective, kind = _classify(original)
        if not kind:
            return
        if kind == _PRIVATE and (
            self._trusted or _normalise_host(host) in self._hosts or any(effective in net for net in self._networks)
        ):
            return
        if kind == _LOOPBACK and (
            self._trusted
            or (_normalise_host(host) == "localhost" and "localhost" in self._hosts)
            or any(effective in net for net in self._loopback)
        ):
            return
        shown = (
            original.ipv4_mapped if isinstance(original, ipaddress.IPv6Address) and original.ipv4_mapped else original
        )
        article = "an" if kind[0] in "aeiou" else "a"
        if host == str(shown) or (_is_ip_literal(host) and _address(host) == original):
            message = f"{shown} is {article} {kind} address"
        else:
            message = f"{host} resolves to {shown}, {article} {kind} address"
        allowable = kind in (_PRIVATE, _LOOPBACK)
        if allowable:
            message += "; only the platform operator can allow it (mcp.allowed_private_hosts)"
        else:
            message += "; MCP servers may never use it"
        raise AddressRefusedError(message, allowable=allowable)

    async def resolve(self, host: str, port: int, timeout: float | None = MCP_DEFAULT_TIMEOUT) -> list[str]:
        """The addresses of host; AddressRefusedError when one of them is refused.

        The lookup is bounded by timeout (the connect timeout of the request):
        httpx.ConnectTimeout when it takes longer.
        """
        if _is_ip_literal(host):
            addresses = [host]
        else:
            try:
                addresses = await asyncio.wait_for(self._lookup(host, port), timeout)
            except TimeoutError:
                msg = f"DNS lookup of {host} timed out after {timeout:g} s"
                raise httpx.ConnectTimeout(msg) from None
            if not addresses:
                raise OSError(f"{host} has no addresses")
        for address in addresses:
            self.check_address(host, address)
        return addresses

    async def _lookup(self, host: str, port: int) -> list[str]:
        if self._resolver is not None:
            return await self._resolver(host, port)
        infos = await asyncio.get_running_loop().getaddrinfo(host, port, type=socket.SOCK_STREAM)
        return list(dict.fromkeys(str(info[4][0]) for info in infos))

    async def check_url(self, url: str, timeout: float = MCP_DEFAULT_TIMEOUT) -> None:
        """Refuse url before anything connects to it (AddressRefusedError, with the reason)."""
        parsed = httpx.URL(url)
        if parsed.scheme not in _DEFAULT_PORTS or not parsed.host:
            raise AddressRefusedError("the url must be an http or https URL with a host")
        await self.resolve(parsed.host, parsed.port or _DEFAULT_PORTS[parsed.scheme], timeout)


class GuardedTransport(httpx.AsyncBaseTransport):
    """Sends each request to a checked address of its host.

    The checked addresses of a host are kept for ``cache_ttl`` seconds, so an
    MCP session does not look its host up for every message; that is safe
    because a request goes only to an address that was checked.
    """

    def __init__(
        self,
        policy: OutboundPolicy,
        inner: httpx.AsyncBaseTransport | None = None,
        *,
        cache_ttl: float = _ADDRESS_CACHE_TTL,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._policy = policy
        self._inner = inner or httpx.AsyncHTTPTransport()
        self._cache_ttl = cache_ttl
        self._clock = clock
        self._cache: dict[tuple[str, int], tuple[float, list[str]]] = {}

    async def _checked_addresses(self, host: str, port: int, timeout: float | None) -> list[str]:
        key = (host.lower(), port)
        cached = self._cache.get(key)
        if cached is not None and self._clock() < cached[0]:
            return cached[1]
        addresses = await self._policy.resolve(host, port, timeout)
        self._cache[key] = (self._clock() + self._cache_ttl, addresses)
        return addresses

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        url = request.url
        if url.scheme not in _DEFAULT_PORTS or not url.host:
            raise AddressRefusedError(f"refused a {url.scheme} request: only http and https are used")
        timeouts = request.extensions.get("timeout") or {}
        addresses = await self._checked_addresses(
            url.host, url.port or _DEFAULT_PORTS[url.scheme], timeouts.get("connect", MCP_DEFAULT_TIMEOUT)
        )
        extensions = dict(request.extensions)
        if not _is_ip_literal(url.host):
            # TLS verifies the certificate for the host, not for the address.
            extensions["sni_hostname"] = url.host
        # Every address is checked; like Go's DialContext, the next one is tried
        # when one cannot be connected to (nothing of the request was sent yet).
        for address in addresses[:-1]:
            try:
                return await self._send_to(request, address, extensions)
            except (httpx.ConnectError, httpx.ConnectTimeout):
                continue
        return await self._send_to(request, addresses[-1], extensions)

    async def _send_to(self, request: httpx.Request, address: str, extensions: dict[str, object]) -> httpx.Response:
        pinned = httpx.Request(
            request.method,
            request.url.copy_with(host=address),
            headers=request.headers,  # carries the Host header of the url
            stream=request.stream,
            extensions=extensions,
        )
        return await self._inner.handle_async_request(pinned)

    async def aclose(self) -> None:
        await self._inner.aclose()


def guarded_client_factory(policy: OutboundPolicy) -> McpHttpClientFactory:
    """An MCP SDK client factory whose clients connect only through GuardedTransport.

    The clients ignore proxy settings of the environment (a proxy would choose the
    address the policy checks); otherwise they match the SDK's default client.
    """

    def create(
        headers: dict[str, str] | None = None,
        timeout: httpx.Timeout | None = None,
        auth: httpx.Auth | None = None,
    ) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            headers=headers,
            timeout=timeout
            if timeout is not None
            else httpx.Timeout(MCP_DEFAULT_TIMEOUT, read=MCP_DEFAULT_SSE_READ_TIMEOUT),
            auth=auth,
            transport=GuardedTransport(policy),
            trust_env=False,
        )

    return create
