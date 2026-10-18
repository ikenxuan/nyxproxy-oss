package monitoring

import (
	"testing"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
)

func TestMonitoringStartReturnsAndStopIsIdempotent(t *testing.T) {
	cfg := &config.ProxyConfig{}
	cfg.Monitoring.Enabled = true
	cfg.Monitoring.Port = 0

	srv := New(cfg, metrics.New("test"), "test")
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.Addr() == "" {
		t.Fatal("Addr is empty")
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
