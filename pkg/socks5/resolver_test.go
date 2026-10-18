package socks5

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestBuildDNSDialerUsesFreshSourceAddressPerDial(t *testing.T) {
	first := net.ParseIP("2001:db8::1")
	second := net.ParseIP("2001:db8::2")

	sourceCalls := 0
	getLocalAddr := func() net.IP {
		sourceCalls++
		if sourceCalls == 1 {
			return first
		}
		return second
	}

	var seen []net.IP
	dial := func(_ context.Context, _ string, laddr *net.UDPAddr, _ string) (net.Conn, error) {
		if laddr == nil {
			seen = append(seen, nil)
		} else {
			seen = append(seen, laddr.IP)
		}
		return nil, errors.New("unreachable in test")
	}

	dnsDial := buildDNSDialer(getLocalAddr, 6, dial)

	if _, err := dnsDial(context.Background(), "udp", "ignored:53"); err == nil {
		t.Fatal("expected all DNS servers to fail")
	}
	// The source address must be re-evaluated for the next query instead of
	// staying pinned to the address that may have just been rotated away.
	if _, err := dnsDial(context.Background(), "udp", "ignored:53"); err == nil {
		t.Fatal("expected all DNS servers to fail")
	}

	if sourceCalls != 2 {
		t.Fatalf("expected source address to be evaluated once per DNS dial, got %d", sourceCalls)
	}
	if len(seen) != 6 {
		t.Fatalf("expected 3 DNS servers per dial (6 total), got %d", len(seen))
	}
	for i := 0; i < 3; i++ {
		if !seen[i].Equal(first) {
			t.Fatalf("dial %d used %v, want %s", i, seen[i], first)
		}
	}
	for i := 3; i < 6; i++ {
		if !seen[i].Equal(second) {
			t.Fatalf("dial %d used %v, want %s", i, seen[i], second)
		}
	}
}

func TestBuildDNSDialerWithoutSourceAddress(t *testing.T) {
	var laddr *net.UDPAddr = &net.UDPAddr{IP: net.ParseIP("192.0.2.1")}
	dial := func(_ context.Context, _ string, got *net.UDPAddr, _ string) (net.Conn, error) {
		laddr = got
		return nil, errors.New("unreachable in test")
	}

	dnsDial := buildDNSDialer(nil, 4, dial)
	if _, err := dnsDial(context.Background(), "udp", "ignored:53"); err == nil {
		t.Fatal("expected all DNS servers to fail")
	}
	if laddr != nil {
		t.Fatalf("expected no local address, got %v", laddr)
	}
}
