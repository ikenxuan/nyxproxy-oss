package network

import (
	"net"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	return nil, f.err
}

func newTestManager(runner CommandRunner, poolSize int) *IPv6Manager {
	_, subnet, _ := net.ParseCIDR("2001:db8:1:2::/64")
	return &IPv6Manager{
		interfaceName: "eth0",
		subnet:        subnet,
		ipPool:        make([]net.IP, 0),
		ipStats:       make(map[string]*IPStats),
		maxUsageCount: 100,
		maxAge:        30 * time.Minute,
		rotationStop:  make(chan bool),
		runner:        runner,
		poolSize:      poolSize,
	}
}

func TestAssignIPUsesInjectedRunner(t *testing.T) {
	runner := &fakeRunner{}
	mgr := newTestManager(runner, 1)
	ip := net.ParseIP("2001:db8:1:2::abcd")

	if err := mgr.assignIPToInterface(ip); err != nil {
		t.Fatalf("assignIPToInterface: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	got := strings.Join(runner.calls[0], " ")
	want := "ip -6 addr add 2001:db8:1:2::abcd/128 dev eth0"
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

func TestRemoveIPUsesInjectedRunner(t *testing.T) {
	runner := &fakeRunner{}
	mgr := newTestManager(runner, 1)

	if err := mgr.removeIPFromInterface("2001:db8:1:2::abcd"); err != nil {
		t.Fatalf("removeIPFromInterface: %v", err)
	}
	got := strings.Join(runner.calls[0], " ")
	want := "ip -6 addr del 2001:db8:1:2::abcd/128 dev eth0"
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

func TestPopulatePoolAddsEveryGeneratedAddress(t *testing.T) {
	runner := &fakeRunner{}
	mgr := newTestManager(runner, 3)

	if err := mgr.populateIPPool(3); err != nil {
		t.Fatalf("populateIPPool: %v", err)
	}
	if mgr.GetPoolSize() != 3 {
		t.Fatalf("pool size = %d, want 3", mgr.GetPoolSize())
	}
	if len(runner.calls) != 3 {
		t.Fatalf("add commands = %d, want 3", len(runner.calls))
	}
	for i, call := range runner.calls {
		if call[0] != "ip" || call[3] != "add" {
			t.Fatalf("call %d = %v", i, call)
		}
	}
}

func TestStopRemovesPoolAddressesAndIsIdempotent(t *testing.T) {
	runner := &fakeRunner{}
	mgr := newTestManager(runner, 2)
	if err := mgr.populateIPPool(2); err != nil {
		t.Fatalf("populateIPPool: %v", err)
	}
	added := len(runner.calls)
	mgr.Stop()
	mgr.Stop()

	deleted := 0
	for _, call := range runner.calls[added:] {
		if len(call) > 3 && call[3] == "del" {
			deleted++
		}
	}
	if deleted != 2 {
		t.Fatalf("delete commands = %d, want 2; calls=%v", deleted, runner.calls)
	}
}

func TestPrefixReturnsCIDR(t *testing.T) {
	mgr := newTestManager(&fakeRunner{}, 0)
	if got := mgr.Prefix(); got != "2001:db8:1:2::/64" {
		t.Fatalf("Prefix() = %q", got)
	}
}

func TestSuggestedPrefix(t *testing.T) {
	cases := []struct {
		cidr string
		want string
	}{
		{"2001:db8:1:2::1/64", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::1/128", ""},
		{"192.0.2.1/24", ""},
	}
	for _, tc := range cases {
		_, ipNet, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatal(err)
		}
		if got := suggestedPrefix(ipNet); got != tc.want {
			t.Fatalf("suggestedPrefix(%s) = %q, want %q", tc.cidr, got, tc.want)
		}
	}
}

func TestDetectSubnetFromDetails(t *testing.T) {
	details := []InterfaceDetails{
		{Name: "eth0", SuggestedPrefix: "2001:db8:1:2::/64"},
		{Name: "eth1"},
	}
	if got := detectSubnetFromDetails("eth0", details); got != "2001:db8:1:2::/64" {
		t.Fatalf("detected subnet = %q", got)
	}
	if got := detectSubnetFromDetails("eth9", details); got != "" {
		t.Fatalf("unknown interface returned %q", got)
	}
}
