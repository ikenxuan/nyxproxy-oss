package network

import (
	"net"
	"testing"
)

func TestSelectDNSAddressSkipsPooledAddresses(t *testing.T) {
	pooled := net.ParseIP("2001:db8::abcd")
	stable := net.ParseIP("2001:db8::1")

	got, err := SelectDNSAddress(
		[]net.IP{pooled, stable},
		6,
		func(ip net.IP) bool { return ip.Equal(pooled) },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(stable) {
		t.Fatalf("expected stable address %s, got %s", stable, got)
	}
}

func TestSelectDNSAddressFallsBackWhenAllAddressesArePooled(t *testing.T) {
	pooled := net.ParseIP("2001:db8::abcd")

	got, err := SelectDNSAddress(
		[]net.IP{pooled},
		6,
		func(ip net.IP) bool { return true },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(pooled) {
		t.Fatalf("expected pooled fallback %s, got %s", pooled, got)
	}
}

func TestSelectDNSAddressSkipsLinkLocalAndLoopback(t *testing.T) {
	global := net.ParseIP("2001:db8::1")

	got, err := SelectDNSAddress(
		[]net.IP{net.ParseIP("fe80::1"), net.ParseIP("::1"), global},
		6,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(global) {
		t.Fatalf("expected global address %s, got %s", global, got)
	}
}

func TestSelectDNSAddressProtocol4(t *testing.T) {
	want := net.ParseIP("192.0.2.10")

	got, err := SelectDNSAddress(
		[]net.IP{net.ParseIP("2001:db8::1"), want},
		4,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("expected IPv4 address %s, got %s", want, got)
	}
}

func TestSelectDNSAddressNoSuitableAddress(t *testing.T) {
	if _, err := SelectDNSAddress([]net.IP{net.ParseIP("fe80::1")}, 6, nil); err == nil {
		t.Fatal("expected an error when no suitable address exists")
	}
}
