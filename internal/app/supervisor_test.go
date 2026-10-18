package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

type fakeProxy struct {
	cfg       config.ProxyConfig
	startErr  error
	startHook func()
	pool      network.Pool
	done      chan struct{}
	mu        sync.Mutex
	state     proxy.State
	stopped   bool
}

func newFakeProxy(cfg *config.ProxyConfig) *fakeProxy {
	return &fakeProxy{cfg: *cfg, done: make(chan struct{}), state: proxy.StateStarting}
}

func (f *fakeProxy) Start() error {
	if f.startHook != nil {
		f.startHook()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		f.state = proxy.StateError
		return f.startErr
	}
	f.state = proxy.StateRunning
	return nil
}

func (f *fakeProxy) Wait() error {
	<-f.done
	return nil
}

func (f *fakeProxy) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped {
		f.stopped = true
		close(f.done)
	}
	f.state = proxy.StateStopped
	return nil
}

// exit simulates the proxy ending on its own (for example after a fatal
// runtime error) without going through Supervisor.Stop.
func (f *fakeProxy) exit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped {
		f.stopped = true
		close(f.done)
	}
}

func (f *fakeProxy) Status() proxy.ProxyStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return proxy.ProxyStatus{State: f.state, Type: f.cfg.GetProxyType()}
}

func (f *fakeProxy) IPv6Manager() network.Pool { return f.pool }

// fakePool is an in-memory network.Pool used to assert that the supervisor
// mirrors rotation statistics into the metrics collector.
type fakePool struct {
	mu      sync.Mutex
	size    int
	rotated int64
	prefix  string
}

func (p *fakePool) GetRandomIPv6() (net.IP, error) { return nil, errors.New("unused") }
func (p *fakePool) IsPoolAddress(net.IP) bool      { return false }
func (p *fakePool) Stop()                          {}
func (p *fakePool) Prefix() string {
	if p.prefix != "" {
		return p.prefix
	}
	return "2001:db8::/64"
}

func (p *fakePool) GetPoolSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size
}

func (p *fakePool) GetTotalRotated() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rotated
}

func (p *fakePool) Snapshot() network.PoolSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return network.PoolSnapshot{
		Size:          p.size,
		Prefix:        p.Prefix(),
		Rotations:     p.rotated,
		MaxUsage:      10,
		MaxAgeSeconds: 600,
	}
}

func (p *fakePool) set(size int, rotated int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = size
	p.rotated = rotated
}

type scriptedFactory struct {
	mu       sync.Mutex
	calls    int
	failOn   int
	created  []*fakeProxy
	onCreate func(*fakeProxy)
}

func (f *scriptedFactory) create(cfg *config.ProxyConfig) (proxy.Proxy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	p := newFakeProxy(cfg)
	if f.onCreate != nil {
		f.onCreate(p)
	}
	if f.calls == f.failOn {
		p.startErr = errors.New("boom")
	}
	f.created = append(f.created, p)
	return p, nil
}

func baseConfig() config.ProxyConfig {
	return config.ProxyConfig{
		Proxy: config.ProxySettings{
			Type:          "socks5",
			ListenAddress: "127.0.0.1",
			ListenPort:    1080,
			Username:      "admin",
			Password:      "secret",
		},
		Network: config.NetworkSettings{
			IPv4Enabled:  true,
			RotateIPv6:   false,
			IPv6PoolSize: 4,
			IPv6MaxUsage: 10,
			IPv6MaxAge:   30,
		},
		Monitoring: config.MonitoringSettings{Enabled: false},
		Logging:    config.LoggingSettings{DebugLevel: 1},
		Web:        config.WebSettings{Enabled: false, ListenAddress: "127.0.0.1", Port: 18081, Username: "admin"},
	}
}

func newTestStore(t *testing.T, cfg config.ProxyConfig) (*config.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(renderTestConfig(t, cfg)), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, path
}

func renderTestConfig(t *testing.T, cfg config.ProxyConfig) string {
	t.Helper()
	return strings.Join([]string{
		"proxy:",
		"  type: " + cfg.Proxy.Type,
		"  listen_address: " + cfg.Proxy.ListenAddress,
		"  listen_port: 1080",
		"  username: " + cfg.Proxy.Username,
		"  password: " + cfg.Proxy.Password,
		"network:",
		"  ipv4_enabled: true",
		"  ipv6_enabled: false",
		"  rotate_ipv6: false",
		"monitoring:",
		"  enabled: false",
		"logging:",
		"  debug_level: 1",
		"web:",
		"  enabled: false",
		"  listen_address: 127.0.0.1",
		"  port: 18081",
		"  username: admin",
	}, "\n") + "\n"
}

func startSupervisor(t *testing.T, factory *scriptedFactory) (*Supervisor, *config.Store, string) {
	t.Helper()
	cfg := baseConfig()
	store, path := newTestStore(t, cfg)
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})
	return sup, store, path
}

func TestApplyRestartsProxyAndPersists(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, path := startSupervisor(t, factory)

	cfg, _ := store.Snapshot()
	submitted := cfg
	submitted.Proxy.ListenPort = 1081

	result, err := sup.Apply(context.Background(), submitted)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed || !result.Restarted.Proxy {
		t.Fatalf("result = %+v", result)
	}
	if result.Generation != 2 {
		t.Fatalf("generation = %d, want 2", result.Generation)
	}
	if factory.calls != 2 {
		t.Fatalf("factory calls = %d, want 2", factory.calls)
	}
	if !factory.created[0].stopped {
		t.Fatal("old proxy was not stopped")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "listen_port: 1081") {
		t.Fatalf("config not persisted:\n%s", data)
	}
	got, gen := store.Snapshot()
	if got.Proxy.ListenPort != 1081 || gen != 2 {
		t.Fatalf("snapshot = %+v gen=%d", got.Proxy, gen)
	}
}

func TestApplyRollsBackWhenNewProxyFails(t *testing.T) {
	factory := &scriptedFactory{failOn: 2}
	sup, store, path := startSupervisor(t, factory)

	cfg, _ := store.Snapshot()
	submitted := cfg
	submitted.Proxy.ListenPort = 1081

	_, err := sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) {
		t.Fatalf("error = %T %v, want *ApplyError", err, err)
	}
	if applyErr.Code != "apply_failed" || !applyErr.RolledBack {
		t.Fatalf("apply error = %+v", applyErr)
	}
	if factory.calls != 3 {
		t.Fatalf("factory calls = %d, want 3 (start, fail, rollback)", factory.calls)
	}
	if sup.Status().Proxy.State != "running" {
		t.Fatalf("proxy state = %q", sup.Status().Proxy.State)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "listen_port: 1080") {
		t.Fatalf("config changed despite rollback:\n%s", data)
	}
	if _, gen := store.Snapshot(); gen != 1 {
		t.Fatalf("generation = %d, want 1", gen)
	}
}

func TestApplyMonitoringFailureKeepsProxyRunning(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	_, portText, _ := net.SplitHostPort(occupied.Addr().String())
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	submitted, _ := store.Snapshot()
	submitted.Monitoring.Enabled = true
	submitted.Monitoring.Port = port

	_, err = sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "apply_failed" {
		t.Fatalf("error = %v, want apply_failed", err)
	}
	status := sup.Status()
	if status.Proxy.State != "running" {
		t.Fatalf("proxy state = %q, want running", status.Proxy.State)
	}
	if status.Monitoring.State != "stopped" {
		t.Fatalf("monitoring state = %q, want stopped after rollback", status.Monitoring.State)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1 (monitoring-only change)", factory.calls)
	}
}

func TestApplyWithNoChangesDoesNotRestart(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	cfg, _ := store.Snapshot()

	result, err := sup.Apply(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Changed {
		t.Fatal("changed = true, want false")
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
}

func TestApplyRejectsLockedField(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	factory := &scriptedFactory{}
	cfg := baseConfig()
	cfg.Network.IPv6Subnet = "2001:db8:ffff::/64"
	store, _ := newTestStore(t, cfg)
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatal(err)
	}

	submitted, _ := store.Snapshot()
	submitted.Network.IPv6Subnet = "2001:db8:eeee::/64"
	_, err := sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "field_locked" {
		t.Fatalf("error = %v", err)
	}
	if applyErr.Fields["network.ipv6_subnet"] == "" {
		t.Fatalf("fields = %#v", applyErr.Fields)
	}
}

func TestConcurrentApplyReturnsInProgress(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	cfg, _ := store.Snapshot()

	sup.applyMu.Lock()
	_, err := sup.Apply(context.Background(), cfg)
	sup.applyMu.Unlock()
	if !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("error = %v, want ErrApplyInProgress", err)
	}
}

func TestRetryStartsProxyAfterStartupError(t *testing.T) {
	factory := &scriptedFactory{failOn: 1}
	cfg := baseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "127.0.0.1"
	cfg.Web.Port = 18081
	cfg.Web.Username = "admin"
	cfg.Web.Password = "web-secret"
	store, _ := newTestStore(t, cfg)

	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start with web enabled must not fail: %v", err)
	}
	if sup.Status().Proxy.State != "error" {
		t.Fatalf("state = %q, want error", sup.Status().Proxy.State)
	}
	if err := sup.Retry(context.Background()); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if sup.Status().Proxy.State != "running" {
		t.Fatalf("state = %q, want running", sup.Status().Proxy.State)
	}
}

func TestRetryRejectsRunningProxy(t *testing.T) {
	factory := &scriptedFactory{}
	sup, _, _ := startSupervisor(t, factory)
	if err := sup.Retry(context.Background()); !errors.Is(err, ErrProxyAlreadyRunning) {
		t.Fatalf("error = %v, want ErrProxyAlreadyRunning", err)
	}
}

func TestApplyAfterShutdownFails(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sup.Shutdown(ctx)
	if state := sup.Status().Proxy.State; state != "stopped" {
		t.Fatalf("proxy state = %q, want stopped", state)
	}

	cfg, _ := store.Snapshot()
	cfg.Proxy.ListenPort = 1081
	_, err := sup.Apply(context.Background(), cfg)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "apply_failed" {
		t.Fatalf("error = %v, want apply_failed", err)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
}

// TestStartHoldsApplyLockDuringProxyStartup guards against an apply racing a
// slow Start: Start must own applyMu for its whole duration so an authenticated
// apply cannot start (and then leak) a second proxy while the first one is
// still starting up.
func TestStartHoldsApplyLockDuringProxyStartup(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseStart := func() { releaseOnce.Do(func() { close(release) }) }

	// Only the very first proxy blocks, so the assertion below reports a clean
	// failure instead of hanging if Start does not hold the apply lock.
	var blockOnce sync.Once
	factory := &scriptedFactory{
		onCreate: func(p *fakeProxy) {
			p.startHook = func() {
				first := false
				blockOnce.Do(func() { first = true })
				if !first {
					return
				}
				close(entered)
				<-release
			}
		},
	}

	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	t.Cleanup(func() {
		releaseStart()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})

	startResult := make(chan error, 1)
	go func() { startResult <- sup.Start() }()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not reach the gated proxy start")
	}

	submitted, _ := store.Snapshot()
	submitted.Proxy.ListenPort = 1081
	if _, err := sup.Apply(context.Background(), submitted); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("Apply during startup = %v, want ErrApplyInProgress", err)
	}
	if err := sup.Retry(context.Background()); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("Retry during startup = %v, want ErrApplyInProgress", err)
	}

	factory.mu.Lock()
	calls := factory.calls
	factory.mu.Unlock()
	if calls != 1 {
		t.Fatalf("factory calls during startup = %d, want 1", calls)
	}

	releaseStart()
	select {
	case err := <-startResult:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not complete after the gate was released")
	}

	sup.mu.Lock()
	current := sup.current
	sup.mu.Unlock()
	factory.mu.Lock()
	created := append([]*fakeProxy(nil), factory.created...)
	factory.mu.Unlock()
	if len(created) != 1 || current != created[0] {
		t.Fatalf("adopted proxy is not the one created by Start (created %d proxies)", len(created))
	}
	if state := sup.Status().Proxy.State; state != "running" {
		t.Fatalf("proxy state = %q, want running", state)
	}
}

func waitForPoolMetrics(t *testing.T, m *metrics.Metrics, size int, rotated int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		stats := m.GetStats()
		if stats.IPPoolSize == size && stats.IPsRotated == rotated {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("metrics = %d/%d, want %d/%d", stats.IPPoolSize, stats.IPsRotated, size, rotated)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPoolMetricsFollowCurrentProxy(t *testing.T) {
	pool := &fakePool{size: 4, rotated: 2}
	var poolOnce sync.Once
	factory := &scriptedFactory{
		onCreate: func(p *fakeProxy) {
			// Only the first proxy owns a rotation pool, so replacing it must
			// reset the exported metrics instead of keeping stale values.
			poolOnce.Do(func() { p.pool = pool })
		},
	}

	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	m := metrics.New("test")
	sup := New(cfg, store, factory.create, m, "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})

	if stats := m.GetStats(); stats.IPPoolSize != 4 || stats.IPsRotated != 2 {
		t.Fatalf("metrics after start = %d/%d, want 4/2", stats.IPPoolSize, stats.IPsRotated)
	}

	pool.set(7, 5)
	sup.refreshPoolMetrics()
	if stats := m.GetStats(); stats.IPPoolSize != 7 || stats.IPsRotated != 5 {
		t.Fatalf("metrics after refresh = %d/%d, want 7/5", stats.IPPoolSize, stats.IPsRotated)
	}

	submitted, _ := store.Snapshot()
	submitted.Proxy.ListenPort = 1081
	if _, err := sup.Apply(context.Background(), submitted); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if stats := m.GetStats(); stats.IPPoolSize != 0 || stats.IPsRotated != 0 {
		t.Fatalf("metrics after replacing the proxy = %d/%d, want 0/0", stats.IPPoolSize, stats.IPsRotated)
	}
}

func TestPoolMetricsRefreshWhenProxyExits(t *testing.T) {
	pool := &fakePool{size: 3, rotated: 1}
	factory := &scriptedFactory{
		onCreate: func(p *fakeProxy) { p.pool = pool },
	}

	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	m := metrics.New("test")
	sup := New(cfg, store, factory.create, m, "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})

	factory.mu.Lock()
	p := factory.created[0]
	factory.mu.Unlock()

	pool.set(5, 4)
	p.exit()
	waitForPoolMetrics(t, m, 5, 4)
}

func TestPoolMetricsResetOnShutdown(t *testing.T) {
	pool := &fakePool{size: 6, rotated: 3}
	factory := &scriptedFactory{
		onCreate: func(p *fakeProxy) { p.pool = pool },
	}

	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	m := metrics.New("test")
	sup := New(cfg, store, factory.create, m, "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if stats := m.GetStats(); stats.IPPoolSize != 6 || stats.IPsRotated != 3 {
		t.Fatalf("metrics after start = %d/%d, want 6/3", stats.IPPoolSize, stats.IPsRotated)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		sup.Shutdown(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not return")
	}
	if stats := m.GetStats(); stats.IPPoolSize != 0 || stats.IPsRotated != 0 {
		t.Fatalf("metrics after shutdown = %d/%d, want 0/0", stats.IPPoolSize, stats.IPsRotated)
	}

	// Shutdown stays idempotent and must not block on the stopped ticker.
	sup.Shutdown(ctx)
}

func TestPoolMetricsTickerUpdates(t *testing.T) {
	pool := &fakePool{size: 2, rotated: 0}
	factory := &scriptedFactory{
		onCreate: func(p *fakeProxy) { p.pool = pool },
	}

	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	m := metrics.New("test")
	sup := New(cfg, store, factory.create, m, "test")
	sup.poolMetricsInterval = 10 * time.Millisecond
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})

	pool.set(9, 4)
	waitForPoolMetrics(t, m, 9, 4)
}

func TestShutdownBeforeStartIsPrompt(t *testing.T) {
	cfg := baseConfig()
	store, _ := newTestStore(t, cfg)
	sup := New(cfg, store, (&scriptedFactory{}).create, metrics.New("test"), "test")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		sup.Shutdown(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown blocked for a supervisor that never started")
	}
}

func TestCheckPrefixRebuildsProxyInAutoMode(t *testing.T) {
	cfg := baseConfig()
	cfg.Network.RotateIPv6 = true
	cfg.Network.IPv6Subnet = ""
	store, _ := newTestStore(t, cfg)

	factory := &scriptedFactory{}
	factory.onCreate = func(p *fakeProxy) {
		p.pool = &fakePool{prefix: p.cfg.Network.IPv6Subnet}
	}
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	sup.prefixInterval = time.Hour // keep the background ticker out of the way
	sup.prefixDebounce = time.Second
	sup.detectPrefix = func(string) ([]string, error) { return []string{"2001:db8:2::/64"}, nil }
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { sup.Shutdown(context.Background()) })

	// The initial proxy represents the pool built from the old prefix.
	factory.created[0].pool = &fakePool{prefix: "2001:db8:1::/64"}

	sup.checkPrefix() // records the pending prefix
	sup.mu.Lock()
	sup.pendingSince = time.Now().Add(-time.Minute)
	sup.mu.Unlock()
	sup.checkPrefix() // debounce elapsed: rebuilds

	if factory.calls != 2 {
		t.Fatalf("factory calls = %d, want 2", factory.calls)
	}
	if !factory.created[0].stopped {
		t.Fatal("old proxy was not stopped")
	}
	if got := factory.created[1].cfg.Network.IPv6Subnet; got != "2001:db8:2::/64" {
		t.Fatalf("new proxy prefix = %q, want 2001:db8:2::/64", got)
	}
	got, _ := store.Snapshot()
	if got.Network.IPv6Subnet != "" {
		t.Fatalf("stored prefix = %q, want auto mode to stay empty", got.Network.IPv6Subnet)
	}
	if sup.cfg.Network.IPv6Subnet != "" {
		t.Fatalf("runtime config prefix = %q, want auto mode to stay empty", sup.cfg.Network.IPv6Subnet)
	}
}

func TestCheckPrefixIgnoresFixedSubnet(t *testing.T) {
	cfg := baseConfig()
	cfg.Network.RotateIPv6 = true
	cfg.Network.IPv6Subnet = "2001:db8:1::/64"
	store, _ := newTestStore(t, cfg)

	factory := &scriptedFactory{}
	factory.onCreate = func(p *fakeProxy) {
		p.pool = &fakePool{prefix: p.cfg.Network.IPv6Subnet}
	}
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	sup.prefixInterval = time.Hour
	calls := 0
	sup.detectPrefix = func(string) ([]string, error) {
		calls++
		return []string{"2001:db8:2::/64"}, nil
	}
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { sup.Shutdown(context.Background()) })

	sup.checkPrefix()
	sup.checkPrefix()

	if calls != 0 {
		t.Fatalf("detector calls = %d, want 0 for a fixed prefix", calls)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
}

func TestCheckPrefixSwitchesToNewlyAppearedPrefix(t *testing.T) {
	cfg := baseConfig()
	cfg.Network.RotateIPv6 = true
	cfg.Network.IPv6Subnet = ""
	store, _ := newTestStore(t, cfg)

	factory := &scriptedFactory{}
	factory.onCreate = func(p *fakeProxy) {
		p.pool = &fakePool{prefix: p.cfg.Network.IPv6Subnet}
	}
	oldPrefix := "2001:db8:1::/64"
	newPrefix := "2001:db8:2::/64"
	prefixes := []string{oldPrefix}
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	sup.prefixInterval = time.Hour
	sup.prefixDebounce = time.Second
	sup.detectPrefix = func(string) ([]string, error) { return prefixes, nil }
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { sup.Shutdown(context.Background()) })
	factory.created[0].pool = &fakePool{prefix: oldPrefix}

	sup.checkPrefix() // learn the old prefix
	// Backdate the old prefix so the new one is clearly newer.
	sup.mu.Lock()
	sup.prefixSeen[oldPrefix] = time.Now().Add(-time.Minute)
	sup.mu.Unlock()

	prefixes = []string{newPrefix, oldPrefix} // both are present during the transition
	sup.checkPrefix()                         // records the pending target
	sup.mu.Lock()
	sup.pendingSince = time.Now().Add(-time.Minute)
	sup.mu.Unlock()
	sup.checkPrefix() // rebuilds

	if factory.calls != 2 {
		t.Fatalf("factory calls = %d, want 2", factory.calls)
	}
	if got := factory.created[1].cfg.Network.IPv6Subnet; got != newPrefix {
		t.Fatalf("new proxy prefix = %q, want %q", got, newPrefix)
	}

	// The old prefix is still present; the supervisor must keep the new one.
	sup.checkPrefix()
	if factory.calls != 2 {
		t.Fatalf("factory calls = %d after settling, want 2", factory.calls)
	}
}
