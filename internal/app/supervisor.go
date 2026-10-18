package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/https"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/monitoring"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/socks5"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/version"
)

const poolCheckInterval = 5 * time.Minute

type Factory func(cfg *config.ProxyConfig) (proxy.Proxy, error)

func DefaultFactory(cfg *config.ProxyConfig) (proxy.Proxy, error) {
	switch cfg.GetProxyType() {
	case "socks5":
		return socks5.NewProxy(cfg)
	case "https":
		return https.NewProxy(cfg)
	default:
		return nil, fmt.Errorf("unsupported proxy type %q", cfg.GetProxyType())
	}
}

type Supervisor struct {
	mu      sync.Mutex
	applyMu sync.Mutex

	store        *config.Store
	cfg          config.ProxyConfig
	factory      Factory
	metrics      *metrics.Metrics
	buildVersion string

	startedAt  time.Time
	current    proxy.Proxy
	proxyState string
	proxyErr   string

	monitoring      *monitoring.Server
	monitoringState string
	monitoringErr   string

	applyInProgress bool
	applyStep       string
	lastAppliedAt   time.Time
	lastApply       LastApply

	shuttingDown bool

	poolMetricsInterval time.Duration
	poolStop            chan struct{}
	poolDone            chan struct{}
}

func New(cfg config.ProxyConfig, store *config.Store, factory Factory, m *metrics.Metrics, buildVersion string) *Supervisor {
	if factory == nil {
		factory = DefaultFactory
	}
	if m == nil {
		m = metrics.New(cfg.Network.InterfaceName)
	}
	return &Supervisor{
		store:               store,
		cfg:                 cfg,
		factory:             factory,
		metrics:             m,
		buildVersion:        buildVersion,
		proxyState:          string(proxy.StateStopped),
		monitoringState:     "stopped",
		poolMetricsInterval: 10 * time.Second,
	}
}

func (s *Supervisor) Start() error {
	// Hold applyMu for the whole startup. A slow proxy start (for example while
	// filling the IPv6 pool) must not race an authenticated Apply/Retry that
	// could start a second proxy which is then overwritten by adoptProxy.
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	if s.isShuttingDown() {
		return context.Canceled
	}

	s.mu.Lock()
	s.startedAt = time.Now()
	cfg := s.cfg
	s.proxyState = string(proxy.StateStarting)
	s.mu.Unlock()

	if cfg.Monitoring.Enabled {
		if err := s.startMonitoring(&cfg); err != nil {
			log.Printf("monitoring server failed to start: %v", err)
			s.mu.Lock()
			s.monitoringState = "error"
			s.monitoringErr = err.Error()
			s.mu.Unlock()
		}
	}

	p, err := s.factory(&cfg)
	if err == nil && p == nil {
		err = errors.New("proxy factory returned nil")
	}
	if err == nil {
		err = p.Start()
	}
	if err != nil {
		if p != nil {
			stopProxy(p)
		}
		s.setProxyError(err)
		if cfg.Web.Enabled {
			log.Printf("proxy failed to start; web console remains available: %v", err)
			// Keep the metrics ticker running: a Retry from the web console may
			// adopt a proxy with a rotation pool later on.
			s.startPoolMetricsTicker()
			return nil
		}
		return err
	}
	if !s.adoptProxy(p) {
		return context.Canceled
	}
	s.startPoolMetricsTicker()
	return nil
}

func (s *Supervisor) Apply(ctx context.Context, submitted config.ProxyConfig) (ApplyResult, error) {
	if !s.applyMu.TryLock() {
		return ApplyResult{}, ErrApplyInProgress
	}
	defer s.applyMu.Unlock()
	if s.isShuttingDown() {
		return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
	}
	s.setApply(true, "validating")
	defer s.setApply(false, "")

	current, _ := s.store.Snapshot()
	merged, err := config.MergeUpdate(current, submitted)
	if err != nil {
		fields := validationFields(err)
		code := "field_locked"
		for _, message := range fields {
			if message == "requires restart" {
				code = "restart_required"
				break
			}
		}
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: code, Message: err.Error(), Fields: fields}
	}
	if err := merged.Validate(); err != nil {
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "validation_failed", Message: "configuration is invalid", Fields: validationFields(err)}
	}

	proxyChanged := current.Proxy != merged.Proxy || current.Network != merged.Network || current.Logging != merged.Logging
	monitoringChanged := current.Monitoring != merged.Monitoring
	reauthRequired := current.Web.Username != merged.Web.Username || current.Web.Password != merged.Web.Password

	if !proxyChanged && !monitoringChanged && !reauthRequired {
		s.mu.Lock()
		p := s.current
		s.mu.Unlock()
		if p == nil || p.Status().State == proxy.StateError {
			s.setApply(true, "starting_proxy")
			if err := s.restartProxy(ctx, &merged); err != nil {
				s.recordFailure(err)
				return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error()}
			}
		}
		return ApplyResult{Changed: false, Generation: s.store.Generation()}, nil
	}

	tmp, err := s.store.Prepare(merged)
	if err != nil {
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "persist_failed", Message: err.Error()}
	}
	committed := false
	defer func() {
		if !committed {
			s.store.Discard(tmp)
		}
	}()

	var oldProxy proxy.Proxy
	if proxyChanged {
		s.setApply(true, "stopping")
		s.mu.Lock()
		oldProxy = s.current
		s.current = nil
		s.proxyState = string(proxy.StateStarting)
		s.proxyErr = ""
		s.mu.Unlock()

		if oldProxy != nil {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = oldProxy.Stop(stopCtx)
			cancel()
		}

		s.setApply(true, "starting_proxy")
		newProxy, err := s.factory(&merged)
		if err == nil && newProxy == nil {
			err = errors.New("proxy factory returned nil")
		}
		if err == nil {
			err = newProxy.Start()
		}
		if err != nil {
			if newProxy != nil {
				stopProxy(newProxy)
			}
			s.setProxyError(err)
			rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
			s.recordFailure(err)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error(), RolledBack: rolledBack}
		}
		if !s.adoptProxy(newProxy) {
			s.recordFailure(context.Canceled)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
		}
	}

	if monitoringChanged {
		s.setApply(true, "starting_monitoring")
		s.stopMonitoring()
		if err := s.startMonitoring(&merged); err != nil {
			s.setMonitoringError(err)
			rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
			s.recordFailure(err)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error(), RolledBack: rolledBack}
		}
	}

	if s.isShuttingDown() {
		s.recordFailure(context.Canceled)
		return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
	}

	s.setApply(true, "committing")
	if err := s.store.Commit(tmp, merged); err != nil {
		rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "persist_failed", Message: err.Error(), RolledBack: rolledBack}
	}
	committed = true

	now := time.Now()
	generation := s.store.Generation()
	s.mu.Lock()
	s.cfg = merged
	s.lastAppliedAt = now
	s.lastApply = LastApply{OK: true, At: now, Generation: generation}
	s.mu.Unlock()
	s.metrics.SetInterface(merged.Network.InterfaceName)

	return ApplyResult{
		Changed:        true,
		Generation:     generation,
		Restarted:      Restarted{Proxy: proxyChanged, Monitoring: monitoringChanged},
		ReauthRequired: reauthRequired,
	}, nil
}

func (s *Supervisor) Retry(ctx context.Context) error {
	if !s.applyMu.TryLock() {
		return ErrApplyInProgress
	}
	defer s.applyMu.Unlock()
	if s.isShuttingDown() {
		return errors.New("server is shutting down")
	}

	s.mu.Lock()
	p := s.current
	state := s.proxyState
	s.mu.Unlock()
	if p != nil && p.Status().State != proxy.StateError {
		return ErrProxyAlreadyRunning
	}
	if p == nil && (state == string(proxy.StateRunning) || state == string(proxy.StateStarting)) {
		return ErrProxyAlreadyRunning
	}

	s.setApply(true, "starting_proxy")
	defer s.setApply(false, "")
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if err := s.restartProxy(ctx, &cfg); err != nil {
		s.recordFailure(err)
		return err
	}
	return nil
}

func (s *Supervisor) restartProxy(ctx context.Context, cfg *config.ProxyConfig) error {
	s.mu.Lock()
	old := s.current
	s.current = nil
	s.mu.Unlock()
	if old != nil {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = old.Stop(stopCtx)
		cancel()
	}

	p, err := s.factory(cfg)
	if err == nil && p == nil {
		err = errors.New("proxy factory returned nil")
	}
	if err == nil {
		err = p.Start()
	}
	if err != nil {
		if p != nil {
			stopProxy(p)
		}
		s.setProxyError(err)
		return err
	}
	if !s.adoptProxy(p) {
		return context.Canceled
	}
	return nil
}

func (s *Supervisor) rollback(oldCfg config.ProxyConfig, proxyChanged, monitoringChanged bool) bool {
	ok := true
	if proxyChanged {
		s.mu.Lock()
		current := s.current
		s.current = nil
		s.mu.Unlock()
		if current != nil {
			stopProxy(current)
		}

		p, err := s.factory(&oldCfg)
		if err == nil && p == nil {
			err = errors.New("proxy factory returned nil")
		}
		if err == nil {
			err = p.Start()
		}
		if err != nil {
			if p != nil {
				stopProxy(p)
			}
			s.setProxyError(fmt.Errorf("rollback failed: %w", err))
			ok = false
		} else if !s.adoptProxy(p) {
			ok = false
		}
	}
	if monitoringChanged {
		s.stopMonitoring()
		if err := s.startMonitoring(&oldCfg); err != nil {
			s.mu.Lock()
			s.monitoringState = "error"
			s.monitoringErr = err.Error()
			s.mu.Unlock()
			ok = false
		}
	}
	return ok
}

func (s *Supervisor) Shutdown(ctx context.Context) {
	// Shutdown is idempotent: setting the flag repeatedly is harmless and a
	// second call finds no components left to stop.
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()

	// Stop the supervisor-owned ticker before tearing components down so no
	// refresh can observe a half-stopped proxy.
	s.stopPoolMetricsTicker()

	// Let an in-flight apply observe the shutdown flag before stopping
	// components it may still be using. The wait is bounded by ctx.
	s.waitForApply(ctx)

	s.mu.Lock()
	p := s.current
	s.current = nil
	s.proxyState = string(proxy.StateStopped)
	srv := s.monitoring
	s.monitoring = nil
	s.monitoringState = "stopped"
	s.mu.Unlock()

	if p != nil {
		_ = p.Stop(ctx)
	}
	if srv != nil {
		_ = srv.Stop()
	}
	s.refreshPoolMetrics()
}

func (s *Supervisor) waitForApply(ctx context.Context) {
	if s.applyMu.TryLock() {
		s.applyMu.Unlock()
		return
	}
	acquired := make(chan struct{})
	go func() {
		s.applyMu.Lock()
		defer s.applyMu.Unlock()
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-ctx.Done():
	}
}

func (s *Supervisor) isShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
}

func (s *Supervisor) Config() (config.ProxyConfig, uint64) {
	return s.store.Snapshot()
}

// poolStatus builds the summary shown on the dashboard and pool page.
func (s *Supervisor) poolStatus(p proxy.Proxy, cfg config.ProxyConfig) PoolStatus {
	pool := PoolStatus{
		Enabled:              cfg.Network.RotateIPv6,
		Prefix:               cfg.Network.IPv6Subnet,
		MaxUsage:             cfg.Network.IPv6MaxUsage,
		MaxAgeMinutes:        cfg.Network.IPv6MaxAge,
		Size:                 cfg.Network.IPv6PoolSize,
		CheckIntervalSeconds: int(poolCheckInterval / time.Second),
	}
	if p == nil {
		return pool
	}
	mgr := p.IPv6Manager()
	if mgr == nil {
		return pool
	}

	snap := mgr.Snapshot()
	pool.Enabled = true
	pool.Size = snap.Size
	pool.Prefix = snap.Prefix
	pool.Rotations = snap.Rotations
	pool.MaxUsage = snap.MaxUsage
	pool.MaxAgeMinutes = int(snap.MaxAgeSeconds / 60)
	pool.TotalUsage = snap.TotalUsage
	for _, address := range snap.Addresses {
		if address.Usage > 0 {
			pool.UsedAddresses++
		}
	}
	pool.FreshAddresses = snap.Size - pool.UsedAddresses
	if snap.Size > 0 {
		pool.AverageUsage = float64(snap.TotalUsage) / float64(snap.Size)
	}
	if snap.Size > 0 && snap.MaxUsage > 0 {
		pool.Utilization = float64(snap.TotalUsage) / float64(snap.Size*snap.MaxUsage)
	}
	pool.LastRotatedAt = snap.LastRotatedAt
	if snap.LastCheckedAt != nil {
		next := snap.LastCheckedAt.Add(poolCheckInterval)
		pool.NextCheckAt = &next
	}
	return pool
}

// PoolDetails returns the full address list for the pool page.
func (s *Supervisor) PoolDetails() PoolDetails {
	s.mu.Lock()
	p := s.current
	cfg := s.cfg
	s.mu.Unlock()

	now := time.Now()
	details := PoolDetails{
		Pool:        s.poolStatus(p, cfg),
		Addresses:   []PoolAddress{},
		GeneratedAt: now,
	}
	if p == nil {
		return details
	}
	mgr := p.IPv6Manager()
	if mgr == nil {
		return details
	}

	snap := mgr.Snapshot()
	maxAge := time.Duration(snap.MaxAgeSeconds) * time.Second
	for _, address := range snap.Addresses {
		remainingUses := snap.MaxUsage - address.Usage
		if remainingUses < 0 {
			remainingUses = 0
		}
		age := now.Sub(address.AddedAt)
		remainingAge := int64((maxAge - age) / time.Second)
		if remainingAge < 0 {
			remainingAge = 0
		}
		details.Addresses = append(details.Addresses, PoolAddress{
			Address:             address.Address,
			Usage:               address.Usage,
			LastUsed:            address.LastUsed,
			AddedAt:             address.AddedAt,
			RemainingUses:       remainingUses,
			RemainingAgeSeconds: remainingAge,
			State:               poolAddressState(address.Usage, snap.MaxUsage, age, maxAge),
		})
	}
	return details
}

func poolAddressState(usage, maxUsage int, age, maxAge time.Duration) string {
	if (maxUsage > 0 && usage >= maxUsage) || (maxAge > 0 && age >= maxAge) {
		return "expired"
	}
	if (maxUsage > 0 && usage*5 >= maxUsage*4) || (maxAge > 0 && age*5 >= maxAge*4) {
		return "near_limit"
	}
	if usage > 0 {
		return "active"
	}
	return "fresh"
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	cfg := s.cfg
	p := s.current
	startedAt := s.startedAt
	proxyState := s.proxyState
	proxyErr := s.proxyErr
	monitoringState := s.monitoringState
	monitoringErr := s.monitoringErr
	lastAppliedAt := s.lastAppliedAt
	lastApply := s.lastApply
	applying := s.applyInProgress
	step := s.applyStep
	s.mu.Unlock()

	ps := ProxyStatus{State: proxyState, Type: cfg.GetProxyType(), Protocol: cfg.GetProxyProtocol(), Error: proxyErr}
	if p != nil {
		st := p.Status()
		ps = ProxyStatus{
			State:    string(st.State),
			Type:     st.Type,
			Listen:   st.Listen,
			Protocol: st.Protocol,
			Error:    st.Error,
		}
		if !st.StartedAt.IsZero() {
			t := st.StartedAt
			ps.StartedAt = &t
		}
	}
	if ps.State == "" {
		ps.State = string(proxy.StateStopped)
	}

	pool := s.poolStatus(p, cfg)

	stats := s.metrics.GetStats()
	now := time.Now()
	var uptime int64
	if !startedAt.IsZero() {
		uptime = int64(now.Sub(startedAt).Seconds())
	}
	status := Status{
		Version:       s.versionString(),
		UptimeSeconds: uptime,
		Proxy:         ps,
		Monitoring: MonitoringStatus{
			Enabled: cfg.Monitoring.Enabled,
			State:   monitoringState,
			Error:   monitoringErr,
		},
		Pool: pool,
		Metrics: MetricsStatus{
			ActiveConnections: stats.ActiveConnections,
			TotalConnections:  stats.TotalConnections,
			TotalRequests:     stats.TotalRequests,
			BytesSent:         stats.BytesSent,
			BytesReceived:     stats.BytesReceived,
		},
		Config: ConfigStatus{
			Path:       s.store.Path(),
			Generation: s.store.Generation(),
			LastApply:  lastApply,
		},
		Platform: PlatformStatus{
			OS:        runtime.GOOS,
			IPCommand: hasIPCommand(),
			Warnings:  []string{},
		},
		Apply: ApplyStatus{InProgress: applying, Step: step},
	}
	if !lastAppliedAt.IsZero() {
		t := lastAppliedAt
		status.Config.LastAppliedAt = &t
	}

	if !status.Platform.IPCommand {
		status.Platform.Warnings = append(status.Platform.Warnings, "the ip command was not found; IPv6 rotation is unavailable")
	}
	if !lastApply.OK && !lastApply.At.IsZero() {
		status.Platform.Warnings = append(status.Platform.Warnings, "last configuration apply failed: "+lastApply.Error)
	}
	if ps.State == string(proxy.StateError) {
		status.Platform.Warnings = append(status.Platform.Warnings, "proxy is not running: "+ps.Error)
	}
	return status
}

func (s *Supervisor) adoptProxy(p proxy.Proxy) bool {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		stopProxy(p)
		return false
	}
	s.current = p
	s.proxyState = string(proxy.StateRunning)
	s.proxyErr = ""
	s.mu.Unlock()
	// Publish this proxy's pool statistics immediately instead of waiting for
	// the next ticker interval.
	s.refreshPoolMetrics()
	go func() {
		err := p.Wait()
		s.mu.Lock()
		if s.current != p {
			s.mu.Unlock()
			return
		}
		if err != nil {
			s.proxyState = string(proxy.StateError)
			s.proxyErr = err.Error()
		} else {
			s.proxyState = string(proxy.StateStopped)
		}
		s.mu.Unlock()
		// Never call refreshPoolMetrics while holding mu: IPv6Manager itself
		// takes a lock and may be stopped concurrently.
		s.refreshPoolMetrics()
	}()
	return true
}

// refreshPoolMetrics mirrors the current proxy's IPv6 pool statistics into the
// shared metrics collector. When no rotation pool is active it resets both
// values so stale numbers do not survive stop, rollback, or shutdown.
func (s *Supervisor) refreshPoolMetrics() {
	s.mu.Lock()
	p := s.current
	s.mu.Unlock()

	var size int
	var rotated int64
	if p != nil {
		if mgr := p.IPv6Manager(); mgr != nil {
			size = mgr.GetPoolSize()
			rotated = mgr.GetTotalRotated()
		}
	}
	s.metrics.SetIPPoolSize(size)
	s.metrics.SetIPsRotated(rotated)
}

// startPoolMetricsTicker starts the supervisor-owned periodic pool metrics
// refresh. It is a no-op when the ticker already runs or Shutdown has begun, so
// it can never leak a goroutine on a supervisor that is already torn down.
func (s *Supervisor) startPoolMetricsTicker() {
	s.mu.Lock()
	if s.shuttingDown || s.poolStop != nil {
		s.mu.Unlock()
		return
	}
	interval := s.poolMetricsInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	s.poolStop = stop
	s.poolDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.refreshPoolMetrics()
			case <-stop:
				return
			}
		}
	}()
}

// stopPoolMetricsTicker stops the periodic refresh and waits for its goroutine
// to exit. Safe to call multiple times and on a supervisor that never started.
func (s *Supervisor) stopPoolMetricsTicker() {
	s.mu.Lock()
	stop := s.poolStop
	done := s.poolDone
	s.poolStop = nil
	s.poolDone = nil
	s.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	if done != nil {
		<-done
	}
}

func (s *Supervisor) startMonitoring(cfg *config.ProxyConfig) error {
	if !cfg.Monitoring.Enabled {
		s.mu.Lock()
		s.monitoring = nil
		s.monitoringState = "stopped"
		s.monitoringErr = ""
		s.mu.Unlock()
		return nil
	}

	srv := monitoring.New(cfg, s.metrics, version.GetVersionInfo())
	if err := srv.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	s.monitoring = srv
	s.monitoringState = "running"
	s.monitoringErr = ""
	s.mu.Unlock()
	return nil
}

func (s *Supervisor) stopMonitoring() {
	s.mu.Lock()
	srv := s.monitoring
	s.monitoring = nil
	s.monitoringState = "stopped"
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Stop()
	}
}

func (s *Supervisor) setProxyError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proxyState = string(proxy.StateError)
	s.proxyErr = err.Error()
}

func (s *Supervisor) setMonitoringError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.monitoringState = "error"
	s.monitoringErr = err.Error()
}

func (s *Supervisor) setApply(inProgress bool, step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyInProgress = inProgress
	s.applyStep = step
}

func (s *Supervisor) recordFailure(err error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastApply = LastApply{OK: false, Error: err.Error(), At: now, Generation: s.store.Generation()}
}

func (s *Supervisor) versionString() string {
	if s.buildVersion != "" {
		return s.buildVersion
	}
	if version.Commit != "" && version.Commit != "unknown" {
		return version.Commit
	}
	return version.Version
}

func validationFields(err error) map[string]string {
	var verr *config.ValidationError
	if errors.As(err, &verr) {
		return verr.Fields
	}
	return map[string]string{}
}

func hasIPCommand() bool {
	_, err := exec.LookPath("ip")
	return err == nil
}

func stopProxy(p proxy.Proxy) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.Stop(ctx)
}
