package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExampleConfigParses(t *testing.T) {
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatalf("read config.example.yaml: %v", err)
	}

	var cfg ProxyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.example.yaml is not valid YAML: %v", err)
	}
	if cfg.Proxy.Type == "" {
		t.Fatal("proxy.type did not bind from config.example.yaml")
	}
	if cfg.Web.Port != 18081 {
		t.Fatalf("web.port = %d, want 18081", cfg.Web.Port)
	}
}
