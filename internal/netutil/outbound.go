package netutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ErrAddressRefused marks an address an OutboundPolicy refuses (see
// RefusedAddressError).
var ErrAddressRefused = errors.New("address refused")

// Address kinds an OutboundPolicy refuses. kindPrivate is refused only for
// hosts that are not allowlisted.
const (
	kindPrivate     = "private"
	kindLoopback    = "loopback"
	kindLinkLocal   = "link-local"
	kindMetadata    = "cloud metadata"
	kindUnspecified = "unspecified"
	kindMulticast   = "multicast"
	kindReserved    = "reserved"
	kindInvalid     = "invalid"
)

type addrRange struct {
	prefix netip.Prefix
	kind   string
}

// neverAllowed are refused for every host, allowlisted or not: the Go Core's
// and the worker's own services (loopback), cloud metadata endpoints
// (link-local, fd00:ec2::254, 100.100.100.200) and addresses no server
// listens on.
var neverAllowed = []addrRange{
	{netip.MustParsePrefix("0.0.0.0/8"), kindUnspecified},
	{netip.MustParsePrefix("127.0.0.0/8"), kindLoopback},
	{netip.MustParsePrefix("169.254.0.0/16"), kindLinkLocal},
	{netip.MustParsePrefix("100.100.100.200/32"), kindMetadata},
	{netip.MustParsePrefix("224.0.0.0/4"), kindMulticast},
	{netip.MustParsePrefix("240.0.0.0/4"), kindReserved},
	{netip.MustParsePrefix("::/128"), kindUnspecified},
	{netip.MustParsePrefix("::1/128"), kindLoopback},
	{netip.MustParsePrefix("fe80::/10"), kindLinkLocal},
	{netip.MustParsePrefix("fd00:ec2::254/128"), kindMetadata},
	{netip.MustParsePrefix("ff00::/8"), kindMulticast},
}

// morePrivate are private ranges IsPrivateIP does not list: shared address
// space (CGNAT), benchmarking, IETF protocol assignments, IPv6 site-local
// and the NAT64 local-use prefix (RFC 8215; a local translator decides what
// it reaches, and its IPv4 embedding is not the /96 one, so it is not read
// as IPv4).
var morePrivate = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
}

// IPv6 ranges that carry an IPv4 address in their last 32 bits: deprecated
// IPv4-compatible addresses and the NAT64 well-known prefix.
var embedsIPv4 = []netip.Prefix{
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
}

// RefusedAddressError reports an address an OutboundPolicy refuses. It
// matches ErrAddressRefused.
type RefusedAddressError struct {
	Host string     // the host name or IP literal that was connected to
	Addr netip.Addr // the address it resolved to
	Kind string     // "private", "loopback", "link-local", "cloud metadata", ...
}

func (e *RefusedAddressError) Error() string {
	article := "a"
	if strings.ContainsRune("aeiou", rune(e.Kind[0])) {
		article = "an"
	}
	if e.Host == "" || e.Host == e.Addr.String() {
		return fmt.Sprintf("%s is %s %s address", e.Addr, article, e.Kind)
	}
	return fmt.Sprintf("%s resolves to %s, %s %s address", e.Host, e.Addr, article, e.Kind)
}

// Is makes errors.Is(err, ErrAddressRefused) hold.
func (e *RefusedAddressError) Is(target error) bool { return target == ErrAddressRefused }

// Allowable reports whether the address is refused only because it is
// private: the platform operator can allowlist its host.
func (e *RefusedAddressError) Allowable() bool { return e.Kind == kindPrivate }

// OutboundPolicy decides which addresses an outbound connection to a URL that
// a tenant supplied may reach (KI-100). Loopback, link-local (cloud
// metadata), unspecified, multicast and reserved addresses are always
// refused; private ones (RFC 1918, ULA, CGNAT, NAT64 local-use, ...) only
// when neither the host name nor the address is on the operator's allowlist. IPv4 addresses
// inside IPv6 (mapped, compatible, NAT64) are judged as IPv4.
type OutboundPolicy struct {
	hosts    map[string]bool
	prefixes []netip.Prefix
	lookup   func(ctx context.Context, host string) ([]netip.Addr, error)
	dial     func(ctx context.Context, network, address string) (net.Conn, error)
}

// OutboundOption adjusts an OutboundPolicy.
type OutboundOption func(*OutboundPolicy)

// WithLookup replaces the DNS lookup (default net.DefaultResolver).
func WithLookup(lookup func(ctx context.Context, host string) ([]netip.Addr, error)) OutboundOption {
	return func(p *OutboundPolicy) { p.lookup = lookup }
}

// WithDial replaces the dialer that connects to a checked "ip:port".
func WithDial(dial func(ctx context.Context, network, address string) (net.Conn, error)) OutboundOption {
	return func(p *OutboundPolicy) { p.dial = dial }
}

// NewOutboundPolicy builds a policy whose private addresses are allowed for
// the given host names (exact, case-insensitive), IP addresses and CIDR
// prefixes. An entry that is none of these is an error.
func NewOutboundPolicy(allowedPrivate []string, opts ...OutboundOption) (*OutboundPolicy, error) {
	p := &OutboundPolicy{
		hosts: make(map[string]bool),
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		dial: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	for _, raw := range allowedPrivate {
		entry := strings.TrimSpace(raw)
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			p.prefixes = append(p.prefixes, unmapPrefix(prefix))
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			addr = addr.WithZone("").Unmap()
			p.prefixes = append(p.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		if !isHostName(entry) {
			return nil, fmt.Errorf("allowed private host %q: want a host name, an IP address or a CIDR prefix", raw)
		}
		p.hosts[normaliseHost(entry)] = true
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// CheckAddr returns a *RefusedAddressError when a connection to host may not
// reach addr, nil when it may.
func (p *OutboundPolicy) CheckAddr(host string, addr netip.Addr) error {
	effective, kind := classify(addr)
	switch kind {
	case "":
		return nil
	case kindPrivate:
		if p.hosts[normaliseHost(host)] {
			return nil
		}
		for _, prefix := range p.prefixes {
			if prefix.Contains(effective) {
				return nil
			}
		}
	}
	return &RefusedAddressError{Host: host, Addr: addr.Unmap(), Kind: kind}
}

// CheckHost resolves host (a name or an IP literal) and checks every address
// it resolves to, so a URL can be refused before anything connects to it. A
// lookup error is returned as it is (it does not match ErrAddressRefused):
// the connection fails, or is checked again when it dials.
func (p *OutboundPolicy) CheckHost(ctx context.Context, host string) error {
	_, err := p.resolve(ctx, strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	return err
}

// resolve returns the addresses of host, or an error when one of them is
// refused (a name that resolves to a public and a private address is
// refused as a whole).
func (p *OutboundPolicy) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	var addrs []netip.Addr
	if addr, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{addr}
	} else {
		if addrs, err = p.lookup(ctx, host); err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no addresses", Name: host, IsNotFound: true}
		}
	}
	for _, addr := range addrs {
		if err := p.CheckAddr(host, addr); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// DialContext resolves the host of address, checks every address it resolves
// to and dials a checked one: the check covers the address that is actually
// connected to, so DNS rebinding and redirects cannot reach a refused one.
func (p *OutboundPolicy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", address, err)
	}
	addrs, err := p.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var dialErr error
	for _, addr := range addrs {
		conn, err := p.dial(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		dialErr = err
	}
	return nil, dialErr
}

// Transport returns an http.Transport (the defaults of http.DefaultTransport)
// that connects only through DialContext and never through a proxy: a proxy
// would choose the address the policy is meant to check.
func (p *OutboundPolicy) Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = p.DialContext
	t.DialTLSContext = nil
	return t
}

// classify returns the address the kind was decided on (IPv4 for an IPv4
// address inside IPv6) and its kind: "" for a public address.
func classify(addr netip.Addr) (effective netip.Addr, kind string) {
	if !addr.IsValid() {
		return addr, kindInvalid
	}
	addr = addr.WithZone("").Unmap()
	if kind := kindOf(addr); kind != "" {
		return addr, kind
	}
	for _, prefix := range embedsIPv4 {
		if prefix.Contains(addr) {
			b := addr.As16()
			v4 := netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
			return v4, kindOf(v4)
		}
	}
	return addr, ""
}

func kindOf(addr netip.Addr) string {
	for _, r := range neverAllowed {
		if r.prefix.Contains(addr) {
			return r.kind
		}
	}
	if IsPrivateIP(net.IP(addr.AsSlice())) {
		return kindPrivate
	}
	for _, prefix := range morePrivate {
		if prefix.Contains(addr) {
			return kindPrivate
		}
	}
	return ""
}

func unmapPrefix(prefix netip.Prefix) netip.Prefix {
	addr := prefix.Addr()
	if addr.Is4In6() && prefix.Bits() >= 96 {
		return netip.PrefixFrom(addr.Unmap(), prefix.Bits()-96).Masked()
	}
	return prefix.Masked()
}

func normaliseHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// isHostName accepts DNS names and container names: letters, digits, "-",
// "_" and dots between labels.
func isHostName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
			if !letter && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}
