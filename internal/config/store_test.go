package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeBaseConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: "127.0.0.1"
  listen_port: 1080
  username: admin
  password: secret
network:
  ipv4_enabled: true
  ipv6_enabled: false
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStorePrepareCommitUpdatesGeneration(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	cfg, gen := store.Snapshot()
	if gen != 1 {
		t.Fatalf("generation = %d, want 1", gen)
	}
	cfg.Proxy.ListenPort = 1081

	tmp, err := store.Prepare(cfg)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat temp: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("temp mode = %o, want 0600", info.Mode().Perm())
	}
	if err := store.Commit(tmp, cfg); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, gen := store.Snapshot()
	if gen != 2 || got.Proxy.ListenPort != 1081 {
		t.Fatalf("snapshot = %+v gen=%d", got.Proxy, gen)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "listen_port: 1081") {
		t.Fatalf("persisted config missing new port:\n%s", data)
	}
}

func TestStoreDoesNotPersistRuntimeOverrides(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	t.Setenv("NYXPROXY_INTERFACE", "eth9")

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
  interface_name: eth0
  ipv4_enabled: true
  ipv6_enabled: false
  ipv6_subnet: 2001:db8:1::/64
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	effective, _ := store.Snapshot()
	if effective.Network.IPv6Subnet != "2001:db8:ffff::/64" || effective.Network.InterfaceName != "eth9" {
		t.Fatalf("effective config = %+v", effective.Network)
	}

	tmp, err := store.Prepare(effective)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := store.Commit(tmp, effective); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved ProxyConfig
	if err := yaml.Unmarshal(persisted, &saved); err != nil {
		t.Fatalf("parse persisted config: %v", err)
	}
	if saved.Network.IPv6Subnet != "2001:db8:1::/64" {
		t.Fatalf("persisted subnet = %q, want the file value 2001:db8:1::/64", saved.Network.IPv6Subnet)
	}
	if saved.Network.InterfaceName != "eth0" {
		t.Fatalf("persisted interface = %q, want eth0", saved.Network.InterfaceName)
	}
	if strings.Contains(string(persisted), "2001:db8:ffff::/64") || strings.Contains(string(persisted), "eth9") {
		t.Fatalf("runtime overrides leaked into the file:\n%s", persisted)
	}
}

func TestStoreDiscardRemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	cfg, _ := store.Snapshot()

	tmp, err := store.Prepare(cfg)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	store.Discard(tmp)
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists: %v", err)
	}
}

func TestStoreCleansStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	stale := filepath.Join(dir, "config.yaml.tmp-123456")
	if err := os.WriteFile(stale, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStore(path); err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp file was not removed: %v", err)
	}
}
