package config

import (
	"errors"
	"testing"
)

func validBaseConfig() *ProxyConfig {
	return &ProxyConfig{
		Proxy: ProxySettings{
			Type:          "socks5",
			ListenAddress: "0.0.0.0",
			ListenPort:    1080,
			Username:      "admin",
			Password:      "secret",
		},
		Network: NetworkSettings{
			IPv6Enabled: true,
		},
	}
}

func TestValidateReturnsFieldErrors(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Proxy.Type = "ftp"

	err := cfg.Validate()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["proxy.type"] == "" {
		t.Fatalf("missing proxy.type field error: %#v", verr.Fields)
	}
}

func TestValidateDNSSourceAddressAcceptsMatchingIPv6(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Network.DNSSourceAddress = "2001:db8::1"

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDNSSourceAddressRejectsWrongProtocol(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Network.IPv6Enabled = false
	cfg.Network.IPv4Enabled = true
	cfg.Network.DNSSourceAddress = "2001:db8::1"

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected an error for an IPv6 DNS source in IPv4 mode")
	}
}

func TestValidateDNSSourceAddressRejectsLinkLocal(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Network.DNSSourceAddress = "fe80::1"

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected an error for a link-local DNS source")
	}
}

func TestValidateDNSSourceAddressRejectsMalformedAddress(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Network.DNSSourceAddress = "not-an-ip"

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected an error for a malformed DNS source address")
	}
}
