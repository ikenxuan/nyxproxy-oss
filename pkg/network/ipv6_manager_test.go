package network

import (
	"net"
	"testing"
)

func TestIsPoolAddress(t *testing.T) {
	pooled := net.ParseIP("2001:db8::abcd")
	mgr := &IPv6Manager{
		ipStats: map[string]*IPStats{
			pooled.String(): {usageCount: 1},
		},
	}

	if !mgr.IsPoolAddress(pooled) {
		t.Fatal("expected address to be reported as pooled")
	}
	if mgr.IsPoolAddress(net.ParseIP("2001:db8::1")) {
		t.Fatal("did not expect non-pooled address to be reported as pooled")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	mgr := &IPv6Manager{
		rotationStop: make(chan bool),
		ipStats:      map[string]*IPStats{},
	}

	mgr.Stop()
	mgr.Stop() // must not panic on a double close
}
