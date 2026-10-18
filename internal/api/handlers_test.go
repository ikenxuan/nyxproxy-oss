package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
)

func loginCookieAndCSRF(t *testing.T, srv *Server) (*http.Cookie, string) {
	t.Helper()
	body := `{"username":"admin","password":"web-secret"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", strings.NewReader(body))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return rr.Result().Cookies()[0], payload.CSRF
}

func TestGetConfigRedactsPasswords(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/config", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Config config.ProxyConfig `json:"config"`
		Meta   struct {
			Path         string            `json:"path"`
			Generation   uint64            `json:"generation"`
			Locked       map[string]string `json:"locked"`
			PasswordsSet map[string]bool   `json:"passwords_set"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Config.Proxy.Password != "" || payload.Config.Web.Password != "" {
		t.Fatalf("passwords were not redacted: %+v", payload.Config)
	}
	if !payload.Meta.PasswordsSet["proxy.password"] || !payload.Meta.PasswordsSet["web.password"] {
		t.Fatalf("passwords_set = %#v", payload.Meta.PasswordsSet)
	}
	if payload.Meta.Generation != 1 {
		t.Fatalf("generation = %d", payload.Meta.Generation)
	}
}

func TestPutConfigReturnsAppliedResult(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.applyResult = app.ApplyResult{Changed: true, Generation: 2, Restarted: app.Restarted{Proxy: true}}
	cookie, csrf := loginCookieAndCSRF(t, srv)

	cfg, _ := ctrl.Config()
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result app.ApplyResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Generation != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestPutConfigRequiresCSRF(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(`{}`))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
	var envelope errorEnvelope
	json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.Error.Code != "csrf_failed" {
		t.Fatalf("code = %q", envelope.Error.Code)
	}
}

func TestPutConfigMapsLockedError(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.applyErr = &app.ApplyError{
		Code:    "restart_required",
		Message: "changed fields require a restart",
		Fields:  map[string]string{"web.port": "requires restart"},
	}
	cookie, csrf := loginCookieAndCSRF(t, srv)
	cfg, _ := ctrl.Config()
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope errorEnvelope
	json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.Error.Code != "restart_required" || envelope.Error.Fields["web.port"] == "" {
		t.Fatalf("error = %+v", envelope.Error)
	}
}

func TestInterfacesReturnsJSONArray(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/network/interfaces", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var details []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &details); err != nil {
		t.Fatalf("response is not a JSON array: %v", err)
	}
}

func TestStatusReturnsControllerStatus(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.status = app.Status{Version: "test", Proxy: app.ProxyStatus{State: "running"}}
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var status app.Status
	json.Unmarshal(rr.Body.Bytes(), &status)
	if status.Proxy.State != "running" {
		t.Fatalf("status = %+v", status.Proxy)
	}
}

func TestPoolReturnsControllerDetails(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.poolDetails = app.PoolDetails{
		Pool: app.PoolStatus{Enabled: true, Size: 2, Prefix: "2001:db8::/64"},
		Addresses: []app.PoolAddress{
			{Address: "2001:db8::1", Usage: 3, State: "active"},
		},
	}
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/pool", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var details app.PoolDetails
	if err := json.Unmarshal(rr.Body.Bytes(), &details); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if details.Pool.Size != 2 || len(details.Addresses) != 1 {
		t.Fatalf("details = %+v", details)
	}
}

func TestLogsStreamReplaysBufferedRecords(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	srv.opts.Logs.Write([]byte("hello from the proxy\n"))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs/stream", nil).WithContext(ctx)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not stop")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "hello from the proxy") {
		t.Fatalf("body = %q", body)
	}
	if rr.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("missing X-Accel-Buffering")
	}
}

// TestShutdownStopsOpenLogStream is a regression test for Shutdown hanging
// while a browser holds an SSE connection open: the handler must observe the
// server-wide stop signal even though the request context is never cancelled.
func TestShutdownStopsOpenLogStream(t *testing.T) {
	srv := startableServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	cookie, _ := loginCookieAndCSRF(t, srv)
	srv.opts.Logs.Write([]byte("record before shutdown\n"))

	req, err := http.NewRequest(http.MethodGet, "http://"+srv.Addr()+"/api/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)

	// Do returns once the SSE response headers are flushed, so the handler is
	// live (replaying or waiting in its select) from here on.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open log stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}

	bodyCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(resp.Body)
		bodyCh <- string(data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown with open log stream = %v after %s", err, time.Since(start))
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("Shutdown took %s, want it to complete well before the deadline", elapsed)
	}

	select {
	case body := <-bodyCh:
		if !strings.Contains(body, "record before shutdown") {
			t.Fatalf("body = %q", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("log stream did not close after Shutdown")
	}
}

func TestBuildProxyURL(t *testing.T) {
	cases := []struct {
		name   string
		status app.ProxyStatus
		user   string
		pass   string
		host   string
		want   string
	}{
		{
			name:   "wildcard ipv6 socks5",
			status: app.ProxyStatus{Type: "socks5", Listen: "[::]:18080"},
			user:   "admin",
			pass:   "p@ss word",
			host:   "203.0.113.7",
			want:   "socks5://admin:p%40ss%20word@203.0.113.7:18080",
		},
		{
			name:   "wildcard ipv4 http",
			status: app.ProxyStatus{Type: "https", Listen: "0.0.0.0:8080"},
			user:   "u",
			pass:   "p",
			host:   "example.com",
			want:   "http://u:p@example.com:8080",
		},
		{
			name:   "specific host is kept",
			status: app.ProxyStatus{Type: "socks5", Listen: "127.0.0.1:1080"},
			user:   "u",
			pass:   "p",
			host:   "example.com",
			want:   "socks5://u:p@127.0.0.1:1080",
		},
		{
			name:   "no listener",
			status: app.ProxyStatus{Type: "socks5"},
			user:   "u",
			pass:   "p",
			host:   "example.com",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildProxyURL(tc.status, tc.user, tc.pass, tc.host); got != tc.want {
				t.Fatalf("buildProxyURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequestHost(t *testing.T) {
	cases := map[string]string{
		"example.com:18081":   "example.com",
		"example.com":         "example.com",
		"[2001:db8::1]:18081": "2001:db8::1",
		"[2001:db8::1]":       "2001:db8::1",
	}
	for host, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		req.Host = host
		if got := requestHost(req); got != want {
			t.Fatalf("requestHost(%q) = %q, want %q", host, got, want)
		}
	}
}
