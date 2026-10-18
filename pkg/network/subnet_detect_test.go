package network

import "testing"

func TestParseRotationPrefix(t *testing.T) {
	output := `
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP
    inet6 2001:db8:1:2:aaaa:bbbb:cccc:dddd/64 scope global dynamic mngtmpaddr noprefixroute
       valid_lft 86390sec preferred_lft 86390sec
    inet6 2001:db8:1:2::1234/128 scope global
       valid_lft forever preferred_lft forever
    inet6 2001:db8:1:3::1/64 scope global deprecated dynamic
       valid_lft 100sec preferred_lft 0sec
    inet6 2001:db8:1:4::1/64 scope global temporary dynamic
       valid_lft 600sec preferred_lft 600sec
    inet6 fe80::1/64 scope link
       valid_lft forever preferred_lft forever
`
	if got := parseRotationPrefix(output); got != "2001:db8:1:2::/64" {
		t.Fatalf("parseRotationPrefix = %q, want 2001:db8:1:2::/64", got)
	}
}

func TestParseRotationPrefixSkipsPoolAndUnusableAddresses(t *testing.T) {
	output := `
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP
    inet6 2001:db8:1:2::1234/128 scope global
    inet6 2001:db8:1:2::5678/64 scope global tentative
    inet6 fe80::1/64 scope link
`
	if got := parseRotationPrefix(output); got != "" {
		t.Fatalf("parseRotationPrefix = %q, want empty", got)
	}
}

func TestDetectRotationPrefixWithRunner(t *testing.T) {
	runner := &fakeRunner{output: []byte(`
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP
    inet6 2001:db8:aa:bb:1:2:3:4/64 scope global dynamic
       valid_lft 86390sec preferred_lft 86390sec
`)}
	got, err := detectRotationPrefixWithRunner(runner, "eth0")
	if err != nil {
		t.Fatalf("detectRotationPrefixWithRunner: %v", err)
	}
	if got != "2001:db8:aa:bb::/64" {
		t.Fatalf("prefix = %q, want 2001:db8:aa:bb::/64", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if got := runner.calls[0][0] + " " + runner.calls[0][1] + " " + runner.calls[0][2]; got != "ip -6 addr" {
		t.Fatalf("command = %q", got)
	}
}

func TestDetectRotationPrefixReportsNoAddress(t *testing.T) {
	runner := &fakeRunner{output: []byte("2: eth0: <UP>\n    inet6 fe80::1/64 scope link\n")}
	if _, err := detectRotationPrefixWithRunner(runner, "eth0"); err == nil {
		t.Fatal("expected an error when no global /64 exists")
	}
}

func TestParseRotationPrefixesOrdersByPreferredLifetime(t *testing.T) {
	output := `
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP
    inet6 2001:db8:1:2::1/64 scope global dynamic
       valid_lft 100sec preferred_lft 50sec
    inet6 2001:db8:1:3::1/64 scope global dynamic
       valid_lft 86400sec preferred_lft 86400sec
`
	got := parseRotationPrefixes(output)
	want := []string{"2001:db8:1:3::/64", "2001:db8:1:2::/64"}
	if len(got) != len(want) {
		t.Fatalf("parseRotationPrefixes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseRotationPrefixes = %v, want %v", got, want)
		}
	}
}
