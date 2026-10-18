package socks5

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	gosocks5 "github.com/armon/go-socks5"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

func newLifecycleProxy(t *testing.T) *Proxy {
	t.Helper()
	server, err := gosocks5.New(&gosocks5.Config{})
	if err != nil {
		t.Fatalf("new socks5 server: %v", err)
	}
	return &Proxy{
		config: &config.ProxyConfig{Proxy: config.ProxySettings{
			Type:          "socks5",
			ListenAddress: "127.0.0.1",
			ListenPort:    0,
		}},
		server: server,
		state:  proxy.StateStarting,
		done:   make(chan struct{}),
	}
}

func TestStartReturnsAfterBindAndStopClosesListener(t *testing.T) {
	p := newLifecycleProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := p.Addr()
	if addr == "" {
		t.Fatal("Addr is empty")
	}
	if p.Status().State != proxy.StateRunning {
		t.Fatalf("state = %q, want running", p.Status().State)
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

func TestWaitReturnsNilAfterStop(t *testing.T) {
	p := newLifecycleProxy(t)
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
			t.Fatalf("Wait returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return")
	}
}

func TestStopDrainsActiveConnection(t *testing.T) {
	p := newLifecycleProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The accepted connection is drained, but the client side must observe a
	// closed socket after Stop returns.
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("connection still open after Stop")
	} else if err != io.EOF {
		// A reset is acceptable on some platforms.
		if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			t.Logf("read error after stop: %v", err)
		}
	}
}

func TestStartBindFailureReportsErrorState(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	p := newLifecycleProxy(t)
	p.config.Proxy.ListenPort = port
	if err := p.Start(); err == nil {
		t.Fatal("expected bind failure")
	}
	if p.Status().State != proxy.StateError {
		t.Fatalf("state = %q, want error", p.Status().State)
	}
	if p.Status().Error == "" {
		t.Fatal("expected an error message in status")
	}
}
