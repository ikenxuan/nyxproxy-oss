package socks5

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	gosocks5 "github.com/armon/go-socks5"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

// Proxy represents a SOCKS5 proxy server
type Proxy struct {
	config   *config.ProxyConfig
	server   *gosocks5.Server
	pool     network.Pool
	resolver *net.Resolver

	mu        sync.Mutex
	listener  *trackingListener
	state     proxy.State
	lastError string
	startedAt time.Time
	serveErr  error
	done      chan struct{}
}

// customResolver implements the socks5.NameResolver interface
type customResolver struct {
	resolver *net.Resolver
	protocol int
}

func (r *customResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	// If it's already an IP, convert to IPv6 if needed
	if ip := net.ParseIP(name); ip != nil {
		if r.protocol == 6 {
			if ip.To4() != nil {
				mapped := make(net.IP, 16)
				mapped[10] = 0xff
				mapped[11] = 0xff
				copy(mapped[12:], ip.To4())
				return ctx, mapped, nil
			}
			return ctx, ip, nil
		}
		if ip.To4() != nil {
			return ctx, ip, nil
		}
		return ctx, nil, fmt.Errorf("IPv6 address not allowed in IPv4 mode: %s", name)
	}

	// Resolve using our protocol-specific resolver
	addrs, err := r.resolver.LookupIPAddr(ctx, name)
	if err != nil {
		return ctx, nil, fmt.Errorf("DNS lookup failed for %s: %v", name, err)
	}

	if r.protocol == 6 {
		// Try native IPv6 first
		for _, addr := range addrs {
			if addr.IP.To4() == nil {
				return ctx, addr.IP, nil
			}
		}
		// Fall back to IPv4 mapped to IPv6
		for _, addr := range addrs {
			if addr.IP.To4() != nil {
				mapped := make(net.IP, 16)
				mapped[10] = 0xff
				mapped[11] = 0xff
				copy(mapped[12:], addr.IP.To4())
				return ctx, mapped, nil
			}
		}
	} else {
		// IPv4 mode
		for _, addr := range addrs {
			if addr.IP.To4() != nil {
				return ctx, addr.IP, nil
			}
		}
	}

	return ctx, nil, fmt.Errorf("no suitable IP addresses found for %s", name)
}

// debugLog prints debug messages based on debug level
func (p *Proxy) debugLog(level int, format string, args ...interface{}) {
	if p.config.GetDebugLevel() >= level {
		log.Printf(format, args...)
	}
}

// getRandomIPv6 generates a random IPv6 address within the /64 subnet
func getRandomIPv6(baseIP net.IP) net.IP {
	ip := make(net.IP, len(baseIP))
	copy(ip, baseIP)
	for i := 8; i < 16; i++ {
		ip[i] = byte(rand.Intn(256))
	}
	if ip[15] == 1 { // Avoid ::1
		ip[15] = 2
	}
	return ip
}

// getNextLocalAddr returns the next IPv6 address to use
func (p *Proxy) getNextLocalAddr() (net.IP, error) {
	protocol := p.config.GetProxyProtocol()

	// If IPv6 rotation is enabled and we have an IPv6 pool, use rotating IPs
	if protocol == 6 && p.config.Network.RotateIPv6 && p.pool != nil {
		return p.pool.GetRandomIPv6()
	}

	// Otherwise use the static interface IP
	return network.GetOutboundIP(p.config.Network.InterfaceName, protocol)
}

// dnsDialFunc dials a single DNS server from the given local source address.
type dnsDialFunc func(ctx context.Context, network string, laddr *net.UDPAddr, raddr string) (net.Conn, error)

// buildDNSDialer builds a Resolver.Dial function. getLocalAddr is evaluated on
// every DNS query instead of being captured once at startup, so a rotated or
// removed address cannot break name resolution permanently.
func buildDNSDialer(getLocalAddr func() net.IP, protocol int, dial dnsDialFunc) func(ctx context.Context, network, address string) (net.Conn, error) {
	dnsNetwork := "udp6"
	dnsServers := []string{
		"[2001:4860:4860::8888]:53", // Google
		"[2606:4700:4700::1111]:53", // Cloudflare
		"[2620:fe::fe]:53",          // Quad9
	}
	if protocol != 6 {
		dnsNetwork = "udp4"
		dnsServers = []string{
			"8.8.8.8:53", // Google
			"1.1.1.1:53", // Cloudflare
			"9.9.9.9:53", // Quad9
		}
	}

	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		var laddr *net.UDPAddr
		if getLocalAddr != nil {
			if ip := getLocalAddr(); ip != nil {
				laddr = &net.UDPAddr{IP: ip}
			}
		}

		for _, dns := range dnsServers {
			conn, err := dial(ctx, dnsNetwork, laddr, dns)
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("all DNS servers failed")
	}
}

func dialDNS(ctx context.Context, network string, laddr *net.UDPAddr, raddr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if laddr != nil {
		dialer.LocalAddr = laddr
	}
	return dialer.DialContext(ctx, network, raddr)
}

// createResolver creates a protocol-specific DNS resolver
func createResolver(getLocalAddr func() net.IP, protocol int) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial:     buildDNSDialer(getLocalAddr, protocol, dialDNS),
	}
}

// NewProxy creates a new SOCKS5 proxy server
func NewProxy(cfg *config.ProxyConfig) (*Proxy, error) {
	rand.Seed(time.Now().UnixNano())

	protocol := cfg.GetProxyProtocol()

	// Initialize IPv6 Manager if rotating IPv6 is enabled
	var pool network.Pool
	if cfg.Network.RotateIPv6 && cfg.Network.IPv6Subnet != "" {
		mgr, err := network.NewIPv6Manager(
			cfg.Network.InterfaceName,
			cfg.Network.IPv6Subnet,
			cfg.Network.IPv6PoolSize,
			cfg.Network.IPv6MaxUsage,
			cfg.Network.IPv6MaxAge,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create IPv6 manager: %v", err)
		}
		pool = mgr
		log.Printf("IPv6 rotation enabled for subnet: %s", cfg.Network.IPv6Subnet)
	}

	// DNS must not be pinned to an address that background rotation may remove.
	dnsSource := func() net.IP {
		if cfg.Network.DNSSourceAddress != "" {
			return net.ParseIP(cfg.Network.DNSSourceAddress)
		}

		var isPooled func(net.IP) bool
		if pool != nil {
			isPooled = pool.IsPoolAddress
		}

		ip, err := network.GetDNSOutboundIP(cfg.Network.InterfaceName, protocol, isPooled)
		if err != nil {
			return nil
		}
		return ip
	}

	// Without rotation there is no pool to fall back on, so keep the early
	// startup check for a usable outbound address.
	if pool == nil {
		if _, err := network.GetOutboundIP(cfg.Network.InterfaceName, protocol); err != nil {
			return nil, fmt.Errorf("failed to determine outbound IP: %v", err)
		}
	}

	proxy := &Proxy{
		config:   cfg,
		pool:     pool,
		resolver: createResolver(dnsSource, protocol),
		state:    proxy.StateStarting,
		done:     make(chan struct{}),
	}

	// Create credentials checker
	creds := gosocks5.StaticCredentials{
		cfg.GetUsername(): cfg.GetPassword(),
	}

	auth := gosocks5.UserPassAuthenticator{Credentials: creds}

	conf := &gosocks5.Config{
		AuthMethods: []gosocks5.Authenticator{auth},
		Credentials: creds,
		Resolver: &customResolver{
			resolver: proxy.resolver,
			protocol: protocol,
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			localIP, err := proxy.getNextLocalAddr()
			if err != nil {
				return nil, fmt.Errorf("failed to get local IP: %v", err)
			}

			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to split address: %v", err)
			}

			// Use our custom resolver to get the right IP version
			resolver := &customResolver{
				resolver: proxy.resolver,
				protocol: protocol,
			}
			ctx, resolvedIP, err := resolver.Resolve(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve host %s: %v", host, err)
			}

			// For IPv6 mode, ensure we have a valid IPv6 address
			if protocol == 6 && resolvedIP.To4() != nil {
				// Create IPv4-mapped IPv6 address
				mapped := make(net.IP, 16)
				mapped[10] = 0xff
				mapped[11] = 0xff
				copy(mapped[12:], resolvedIP.To4())
				resolvedIP = mapped
				proxy.debugLog(2, "Mapped IPv4 address to IPv6: %s", resolvedIP.String())
			}

			targetAddr := net.JoinHostPort(resolvedIP.String(), port)
			proxy.debugLog(2, "Using local IP: %s for connection to %s (resolved to %s)", localIP, addr, targetAddr)

			dialer := &net.Dialer{
				LocalAddr: &net.TCPAddr{IP: localIP},
				Timeout:   30 * time.Second,
			}

			// Always use the protocol specified in config
			dialNetwork := "tcp4"
			if protocol == 6 {
				dialNetwork = "tcp6"
			}

			return dialer.DialContext(ctx, dialNetwork, targetAddr)
		},
	}

	server, err := gosocks5.New(conf)
	if err != nil {
		if pool != nil {
			pool.Stop()
		}
		return nil, fmt.Errorf("failed to create SOCKS5 server: %v", err)
	}

	proxy.server = server
	return proxy, nil
}

// Start binds the listener and returns once the proxy is accepting
// connections. Serving happens in the background; use Wait for its result.
func (p *Proxy) Start() error {
	addr := net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	p.debugLog(1, "Starting socks5 proxy on %s (Protocol: IPv%d)", addr, p.config.GetProxyProtocol())

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.setError(err)
		if p.pool != nil {
			p.pool.Stop()
		}
		return fmt.Errorf("failed to create listener: %v", err)
	}

	tracked := newTrackingListener(ln)
	p.mu.Lock()
	p.listener = tracked
	p.state = proxy.StateRunning
	p.startedAt = time.Now()
	p.lastError = ""
	p.mu.Unlock()

	go func() {
		err := p.server.Serve(tracked)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		p.mu.Lock()
		p.serveErr = err
		if err != nil {
			p.state = proxy.StateError
			p.lastError = err.Error()
		}
		close(p.done)
		p.mu.Unlock()
	}()
	return nil
}

// Wait blocks until the serve loop exits and reports its result.
func (p *Proxy) Wait() error {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.serveErr
}

// Stop closes the listener, waits for accepted connections to drain or ctx to
// expire, and releases the IPv6 pool.
func (p *Proxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	ln := p.listener
	if ln == nil {
		p.mu.Unlock()
		return nil
	}
	p.listener = nil
	p.mu.Unlock()

	_ = ln.Close()
	ln.wait(ctx)

	if p.pool != nil {
		p.pool.Stop()
	}

	p.mu.Lock()
	if p.state != proxy.StateError {
		p.state = proxy.StateStopped
	}
	p.mu.Unlock()
	return nil
}

// Status reports the current lifecycle state.
func (p *Proxy) Status() proxy.ProxyStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	started := p.startedAt
	state := p.state
	if state == "" {
		state = proxy.StateStarting
	}
	listen := ""
	if p.listener != nil {
		listen = p.listener.Addr().String()
	} else if p.config != nil {
		listen = net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	}
	protocol := 4
	if p.config != nil {
		protocol = p.config.GetProxyProtocol()
	}
	kind := "socks5"
	if p.config != nil && p.config.GetProxyType() != "" {
		kind = p.config.GetProxyType()
	}
	return proxy.ProxyStatus{
		State:     state,
		Type:      kind,
		Listen:    listen,
		Protocol:  protocol,
		StartedAt: started,
		Error:     p.lastError,
	}
}

// Addr returns the bound listen address, or an empty string when not listening.
func (p *Proxy) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// IPv6Manager returns the IPv6 pool backing this proxy, if any.
func (p *Proxy) IPv6Manager() network.Pool {
	return p.pool
}

func (p *Proxy) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = proxy.StateError
	p.lastError = err.Error()
}
