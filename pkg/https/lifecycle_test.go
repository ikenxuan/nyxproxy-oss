package https

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

func newHTTPSProxy(t *testing.T) *Proxy {
	t.Helper()
	cfg := &config.ProxyConfig{Proxy: config.ProxySettings{
		Type:          "https",
		ListenAddress: "127.0.0.1",
		ListenPort:    0,
	}}
	return &Proxy{
		config: cfg,
		httpServer: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		},
		state: proxy.StateStarting,
		done:  make(chan struct{}),
	}
}

func TestHTTPSStartReturnsAfterBind(t *testing.T) {
	p := newHTTPSProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := p.Addr()
	if addr == "" {
		t.Fatal("Addr is empty")
	}
	if p.Status().State != proxy.StateRunning {
		t.Fatalf("state = %q", p.Status().State)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port was not released: %v", err)
	}
	ln.Close()
}

func TestHTTPSWaitReturnsNilAfterStop(t *testing.T) {
	p := newHTTPSProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return")
	}
}
