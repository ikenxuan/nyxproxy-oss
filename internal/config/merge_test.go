package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromAppliesWebDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: "0.0.0.0"
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

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Web.Enabled {
		t.Fatal("web console must be disabled when the web section is absent")
	}
	if cfg.Web.ListenAddress != "127.0.0.1" {
		t.Fatalf("listen address = %q, want 127.0.0.1", cfg.Web.ListenAddress)
	}
	if cfg.Web.Port != 18081 {
		t.Fatalf("port = %d, want 18081", cfg.Web.Port)
	}
	if cfg.Web.Username != "admin" {
		t.Fatalf("username = %q, want admin", cfg.Web.Username)
	}
}

func TestValidateWebRequiresCredentials(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "127.0.0.1"
	cfg.Web.Port = 18081
	cfg.Web.Username = "admin"

	err := cfg.Validate()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["web.password"] == "" {
		t.Fatalf("missing web.password field error: %#v", verr.Fields)
	}
}

func TestValidateRejectsListenerConflict(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "0.0.0.0"
	cfg.Web.Port = cfg.Proxy.ListenPort
	cfg.Web.Username = "admin"
	cfg.Web.Password = "secret"

	err := cfg.Validate()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["web.port"] == "" {
		t.Fatalf("missing web.port conflict: %#v", verr.Fields)
	}
}

func TestRuntimeOverridesWin(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	t.Setenv("NYXPROXY_INTERFACE", "eth9")

	cfg := validBaseConfig()
	cfg.Network.IPv6Subnet = "2001:db8:1::/64"
	cfg.Network.InterfaceName = "eth0"
	ApplyRuntimeOverrides(cfg)

	if cfg.Network.IPv6Subnet != "2001:db8:ffff::/64" {
		t.Fatalf("subnet = %q", cfg.Network.IPv6Subnet)
	}
	if cfg.Network.InterfaceName != "eth9" {
		t.Fatalf("interface = %q", cfg.Network.InterfaceName)
	}
	locked := LockedFields()
	if locked["network.ipv6_subnet"] != "env" || locked["network.interface_name"] != "env" {
		t.Fatalf("locked fields = %#v", locked)
	}
}

func TestMergeUpdateRejectsLockedFieldChange(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	current := *validBaseConfig()
	current.Network.IPv6Subnet = "2001:db8:ffff::/64"
	submitted := current
	submitted.Network.IPv6Subnet = "2001:db8:eeee::/64"

	_, err := MergeUpdate(current, submitted)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["network.ipv6_subnet"] == "" {
		t.Fatalf("missing locked field error: %#v", verr.Fields)
	}
}

func TestMergeUpdatePreservesPasswords(t *testing.T) {
	current := *validBaseConfig()
	current.Proxy.Password = "old-proxy"
	current.Web.Enabled = true
	current.Web.Username = "admin"
	current.Web.Password = "old-web"

	submitted := current
	submitted.Proxy.Password = ""
	submitted.Web.Password = ""

	merged, err := MergeUpdate(current, submitted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Proxy.Password != "old-proxy" {
		t.Fatalf("proxy password = %q", merged.Proxy.Password)
	}
	if merged.Web.Password != "old-web" {
		t.Fatalf("web password = %q", merged.Web.Password)
	}
}

func TestRedactedConfigHidesPasswords(t *testing.T) {
	cfg := *validBaseConfig()
	cfg.Web.Password = "web-secret"
	red := cfg.Redacted()
	if red.Proxy.Password != "" || red.Web.Password != "" {
		t.Fatalf("passwords were not redacted: %#v", red)
	}
	set := cfg.PasswordsSet()
	if !set["proxy.password"] || !set["web.password"] {
		t.Fatalf("passwords_set = %#v", set)
	}
}
