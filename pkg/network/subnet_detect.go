package network

import (
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
)

// DetectIPv6Subnet automatically detects the IPv6 /64 subnet for an interface
func DetectIPv6Subnet(interfaceName string) (string, error) {
	// If no interface specified, auto-detect
	if interfaceName == "" {
		iface, err := getDefaultInterface()
		if err != nil {
			return "", err
		}
		interfaceName = iface.Name
	}

	// Get the interface
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %v", interfaceName, err)
	}

	// Get addresses
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses: %v", err)
	}

	// Find first global IPv6 address and extract /64 subnet
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP
		// Check if it's IPv6 and not link-local
		if ip.To4() == nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
			// Create /64 subnet from this IP
			subnet := make(net.IP, len(ip))
			copy(subnet, ip)

			// Zero out the last 64 bits (host part)
			for i := 8; i < 16; i++ {
				subnet[i] = 0
			}

			// Return as CIDR notation
			return fmt.Sprintf("%s/64", subnet.String()), nil
		}
	}

	return "", fmt.Errorf("no global IPv6 address found on interface %s", interfaceName)
}

// getDefaultInterface returns the first non-loopback interface with an IPv6 address
func getDefaultInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Check for global IPv6
			if ip.To4() == nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no interface with global IPv6 address found")
}

// GetIPv6SubnetInfo returns detailed information about the IPv6 configuration
func GetIPv6SubnetInfo(interfaceName string) (map[string]string, error) {
	info := make(map[string]string)

	if interfaceName == "" {
		iface, err := getDefaultInterface()
		if err != nil {
			return nil, err
		}
		interfaceName = iface.Name
	}

	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}

	info["interface"] = iface.Name

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	var ipv6Addrs []string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP
		if ip.To4() == nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
			ipv6Addrs = append(ipv6Addrs, ip.String())
		}
	}

	if len(ipv6Addrs) > 0 {
		info["primary_ipv6"] = ipv6Addrs[0]
		info["ipv6_count"] = fmt.Sprintf("%d", len(ipv6Addrs))

		subnet, err := DetectIPv6Subnet(interfaceName)
		if err == nil {
			info["detected_subnet"] = subnet
		}
	}

	return info, nil
}

// NormalizeSubnet ensures the subnet is in correct /64 format
func NormalizeSubnet(subnet string) (string, error) {
	// Parse the CIDR
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet format: %v", err)
	}

	// Check if it's IPv6
	if ipNet.IP.To4() != nil {
		return "", fmt.Errorf("subnet must be IPv6")
	}

	// Get the mask size
	ones, bits := ipNet.Mask.Size()
	if bits != 128 {
		return "", fmt.Errorf("invalid IPv6 subnet")
	}

	// We want /64
	if ones > 64 {
		// If it's more specific than /64, extract the /64
		ip := ipNet.IP.Mask(net.CIDRMask(64, 128))
		return fmt.Sprintf("%s/64", ip.String()), nil
	} else if ones < 64 {
		// If it's less specific (like /48), we can't determine a single /64
		return "", fmt.Errorf("subnet mask /%d is too large, need /64 or more specific", ones)
	}

	// Already /64, just ensure proper formatting
	ip := ipNet.IP.Mask(net.CIDRMask(64, 128))
	return fmt.Sprintf("%s/64", ip.String()), nil
}

// DetectRotationPrefixes returns every usable global /64 on the interface,
// most preferred first. Deprecated, temporary, and tentative addresses are
// ignored; addresses with a longer remaining preferred lifetime sort first so
// a freshly delegated prefix is preferred over one that is expiring.
//
// Pool addresses are added as /128 and are therefore never returned.
func DetectRotationPrefixes(interfaceName string) ([]string, error) {
	return detectRotationPrefixesWithRunner(execRunner{}, interfaceName)
}

// DetectRotationPrefix returns the best usable global /64 on the interface.
func DetectRotationPrefix(interfaceName string) (string, error) {
	prefixes, err := DetectRotationPrefixes(interfaceName)
	if err != nil {
		return "", err
	}
	return prefixes[0], nil
}

func detectRotationPrefixesWithRunner(runner CommandRunner, interfaceName string) ([]string, error) {
	if interfaceName == "" {
		iface, err := getDefaultInterface()
		if err != nil {
			return nil, err
		}
		interfaceName = iface.Name
	}

	output, err := runner.Run("ip", "-6", "addr", "show", "dev", interfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect interface %s: %v: %s",
			interfaceName, err, strings.TrimSpace(string(output)))
	}

	prefixes := parseRotationPrefixes(string(output))
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no usable global IPv6 /64 found on interface %s", interfaceName)
	}
	return prefixes, nil
}

func detectRotationPrefixWithRunner(runner CommandRunner, interfaceName string) (string, error) {
	prefixes, err := detectRotationPrefixesWithRunner(runner, interfaceName)
	if err != nil {
		return "", err
	}
	return prefixes[0], nil
}

// parseRotationPrefix extracts the best usable global /64 from `ip -6 addr
// show` output.
func parseRotationPrefix(output string) string {
	prefixes := parseRotationPrefixes(output)
	if len(prefixes) == 0 {
		return ""
	}
	return prefixes[0]
}

type prefixCandidate struct {
	prefix      string
	preferred   uint64
	hasLifetime bool
	order       int
}

// parseRotationPrefixes extracts usable global /64 prefixes from `ip -6 addr
// show` output, ordered by remaining preferred lifetime.
func parseRotationPrefixes(output string) []string {
	var candidates []*prefixCandidate
	var current *prefixCandidate

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if fields[0] == "inet6" {
			current = nil
			if len(fields) < 4 || !scopeIsGlobal(fields) {
				continue
			}
			if hasAddrFlag(fields, "deprecated") || hasAddrFlag(fields, "temporary") ||
				hasAddrFlag(fields, "tentative") || hasAddrFlag(fields, "dadfailed") {
				continue
			}

			_, ipNet, err := net.ParseCIDR(fields[1])
			if err != nil || ipNet.IP.To4() != nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			if ones != 64 || bits != 128 {
				continue
			}
			current = &prefixCandidate{
				prefix: ipNet.IP.Mask(ipNet.Mask).String() + "/64",
				order:  len(candidates),
			}
			candidates = append(candidates, current)
			continue
		}

		if current != nil {
			for i := 0; i+1 < len(fields); i++ {
				if fields[i] == "preferred_lft" {
					current.preferred = parseLifetime(fields[i+1])
					current.hasLifetime = true
					current = nil
					break
				}
			}
		}
	}

	// Deduplicate prefixes and drop addresses whose preferred lifetime has
	// already expired.
	byPrefix := make(map[string]*prefixCandidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.hasLifetime && candidate.preferred == 0 {
			continue
		}
		if existing, ok := byPrefix[candidate.prefix]; ok {
			if candidate.preferred > existing.preferred {
				existing.preferred = candidate.preferred
			}
			continue
		}
		byPrefix[candidate.prefix] = candidate
	}

	unique := make([]*prefixCandidate, 0, len(byPrefix))
	for _, candidate := range byPrefix {
		unique = append(unique, candidate)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].preferred != unique[j].preferred {
			return unique[i].preferred > unique[j].preferred
		}
		return unique[i].order < unique[j].order
	})

	prefixes := make([]string, 0, len(unique))
	for _, candidate := range unique {
		prefixes = append(prefixes, candidate.prefix)
	}
	return prefixes
}

func parseLifetime(value string) uint64 {
	value = strings.TrimSuffix(value, "sec")
	if value == "forever" {
		return math.MaxUint64
	}
	seconds, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return seconds
}

func scopeIsGlobal(fields []string) bool {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "scope" {
			return fields[i+1] == "global"
		}
	}
	return false
}

func hasAddrFlag(fields []string, flag string) bool {
	for _, field := range fields {
		if field == flag {
			return true
		}
	}
	return false
}

// IsValidIPv6Subnet checks if a string is a valid IPv6 /64 subnet
func IsValidIPv6Subnet(subnet string) bool {
	if !strings.Contains(subnet, "/64") {
		return false
	}

	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return false
	}

	// Must be IPv6
	if ipNet.IP.To4() != nil {
		return false
	}

	ones, bits := ipNet.Mask.Size()
	return ones == 64 && bits == 128
}
