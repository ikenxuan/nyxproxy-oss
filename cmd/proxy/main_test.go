package main

import (
	"testing"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
)

func TestResolveConfigPathPrecedence(t *testing.T) {
	t.Setenv("NYXPROXY_CONFIG", "/env/config.yaml")
	if got := resolveConfigPath("/flag/config.yaml"); got != "/flag/config.yaml" {
		t.Fatalf("flag path = %q", got)
	}
	if got := resolveConfigPath(""); got != "/env/config.yaml" {
		t.Fatalf("env path = %q", got)
	}
	t.Setenv("NYXPROXY_CONFIG", "")
	if got := resolveConfigPath(""); got != "config.yaml" {
		t.Fatalf("default path = %q", got)
	}
}

func TestHealthcheckURLUsesLoopbackForWildcard(t *testing.T) {
	cfg := &config.ProxyConfig{}
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "0.0.0.0"
	cfg.Web.Port = 18081
	if got := healthcheckURL(cfg); got != "http://127.0.0.1:18081/healthz" {
		t.Fatalf("url = %q", got)
	}

	cfg.Web.TLSCertFile = "/certs/tls.crt"
	if got := healthcheckURL(cfg); got != "https://127.0.0.1:18081/healthz" {
		t.Fatalf("tls url = %q", got)
	}

	cfg.Web.ListenAddress = "localhost"
	if got := healthcheckURL(cfg); got != "https://localhost:18081/healthz" {
		t.Fatalf("localhost url = %q", got)
	}
}
