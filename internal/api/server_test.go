package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
)

type fakeController struct {
	status      app.Status
	poolDetails app.PoolDetails
	cfg         config.ProxyConfig
	gen         uint64
	applyResult app.ApplyResult
	applyErr    error
	retryErr    error
}

func (f *fakeController) Status() app.Status                   { return f.status }
func (f *fakeController) PoolDetails() app.PoolDetails         { return f.poolDetails }
func (f *fakeController) Config() (config.ProxyConfig, uint64) { return f.cfg, f.gen }
func (f *fakeController) Apply(ctx context.Context, cfg config.ProxyConfig) (app.ApplyResult, error) {
	return f.applyResult, f.applyErr
}
func (f *fakeController) Retry(ctx context.Context) error { return f.retryErr }

func testServer(t *testing.T) (*Server, *Auth) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: 127.0.0.1
  listen_port: 1080
  username: admin
  password: secret
network:
  ipv4_enabled: true
  ipv6_enabled: false
monitoring:
  enabled: false
logging:
  debug_level: 1
web:
  enabled: true
  listen_address: 127.0.0.1
  port: 18081
  username: admin
  password: web-secret
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := store.Snapshot()
	auth := NewAuth(cfg.Web.Username, cfg.Web.Password, AuthOptions{
		Now:   time.Now,
		Sleep: func(time.Duration) {},
	})
	ctrl := &fakeController{cfg: cfg, gen: 1}
	srv := New(Options{
		Config:     cfg,
		Store:      store,
		Controller: ctrl,
		Auth:       auth,
		Logs:       logbuf.New(10, 2),
		UI:         http.NotFoundHandler(),
	})
	return srv, auth
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security headers")
	}
}

func TestStatusRequiresSession(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "unauthenticated" {
		t.Fatalf("code = %q", envelope.Error.Code)
	}
}

// startableServer returns a server configured to bind an OS-assigned port so
// lifecycle tests never collide with a fixed port on the host.
func startableServer(t *testing.T) *Server {
	t.Helper()
	srv, _ := testServer(t)
	srv.opts.Config.Web.ListenAddress = "127.0.0.1"
	srv.opts.Config.Web.Port = 0
	return srv
}

func TestStartTwiceFails(t *testing.T) {
	srv := startableServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	if err := srv.Start(); err == nil {
		t.Fatal("second Start should fail")
	}
}

func TestWaitAfterFailedStartReturnsError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv, _ := testServer(t)
	srv.opts.Config.Web.ListenAddress = "127.0.0.1"
	srv.opts.Config.Web.Port = ln.Addr().(*net.TCPAddr).Port

	if err := srv.Start(); err == nil {
		t.Fatal("Start should fail when the address is already in use")
	}

	done := make(chan error, 1)
	go func() { done <- srv.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Wait after failed Start = nil, want error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait hung after failed Start")
	}
}

func TestShutdownBeforeStartIsNoop(t *testing.T) {
	srv, _ := testServer(t)
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown before Start: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait hung after Shutdown before Start")
	}

	if err := srv.Start(); err == nil {
		t.Fatal("Start after Shutdown should fail")
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestAddrNonEmptyWhileRunning(t *testing.T) {
	srv := startableServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := srv.Addr(); got == "" {
		t.Fatal("Addr is empty while running")
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if err := srv.Start(); err == nil {
		t.Fatal("Start after Shutdown should fail")
	}
}

func TestLoginSessionAndLogoutFlow(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.Handler()

	loginBody := `{"username":"admin","password":"web-secret"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", strings.NewReader(loginBody))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rr.Code, rr.Body.String())
	}
	cookie := rr.Result().Cookies()[0]

	var login struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/auth/session", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/logout", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", login.CSRFToken)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", rr.Code, rr.Body.String())
	}
}
