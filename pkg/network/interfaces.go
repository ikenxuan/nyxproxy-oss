package network

import (
	"fmt"
	"net"
	"sort"
)

// InterfaceInfo contains information about a network interface
type InterfaceInfo struct {
	Name          string
	Index         int
	IPv4Addresses []string
	IPv6Addresses []string
	IsUp          bool
	IsLoopback    bool
}

// GetAllInterfaces returns all network interfaces on the system
func GetAllInterfaces() ([]InterfaceInfo, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %v", err)
	}

	var result []InterfaceInfo

	for _, iface := range interfaces {
		info := InterfaceInfo{
			Name:       iface.Name,
			Index:      iface.Index,
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		// Get IP addresses for this interface
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
			if ip.To4() != nil {
				info.IPv4Addresses = append(info.IPv4Addresses, ip.String())
			} else {
				// Filter out link-local IPv6 addresses for cleaner display
				if !ip.IsLinkLocalUnicast() {
					info.IPv6Addresses = append(info.IPv6Addresses, ip.String())
				}
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// GetUsableInterfaces returns only interfaces that are up and not loopback
func GetUsableInterfaces() ([]InterfaceInfo, error) {
	all, err := GetAllInterfaces()
	if err != nil {
		return nil, err
	}

	var usable []InterfaceInfo
	for _, iface := range all {
		if iface.IsUp && !iface.IsLoopback {
			// Only include interfaces with at least one IP address
			if len(iface.IPv4Addresses) > 0 || len(iface.IPv6Addresses) > 0 {
				usable = append(usable, iface)
			}
		}
	}

	return usable, nil
}

// GetInterfaceByName returns information about a specific interface by name
func GetInterfaceByName(name string) (*InterfaceInfo, error) {
	all, err := GetAllInterfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range all {
		if iface.Name == name {
			return &iface, nil
		}
	}

	return nil, fmt.Errorf("interface %s not found", name)
}

// GetOutboundIP returns the preferred outbound IP for the given interface and protocol
func GetOutboundIP(interfaceName string, protocol int) (net.IP, error) {
	addrs, err := listInterfaceAddresses(interfaceName)
	if err != nil {
		return nil, err
	}

	return SelectDNSAddress(addrs, protocol, nil)
}

// GetDNSOutboundIP returns an address suitable as the source of DNS queries.
// Addresses reported by isPooled are avoided whenever a stable address exists,
// because the rotation goroutine can delete pool addresses at any time.
func GetDNSOutboundIP(interfaceName string, protocol int, isPooled func(net.IP) bool) (net.IP, error) {
	addrs, err := listInterfaceAddresses(interfaceName)
	if err != nil {
		return nil, err
	}

	return SelectDNSAddress(addrs, protocol, isPooled)
}

// SelectDNSAddress picks the best address for DNS queries from addrs.
//
// Pooled addresses are skipped so that background IP rotation cannot delete the
// address a resolver is pinned to. If every suitable address is pooled, the
// first suitable address is returned as a fallback: callers re-select on every
// DNS query, so a later query can recover once a stable address is available.
func SelectDNSAddress(addrs []net.IP, protocol int, isPooled func(net.IP) bool) (net.IP, error) {
	var fallback net.IP

	for _, ip := range addrs {
		if !isSuitableOutboundIP(ip, protocol) {
			continue
		}
		if fallback == nil {
			fallback = ip
		}
		if isPooled != nil && isPooled(ip) {
			continue
		}
		return ip, nil
	}

	if fallback != nil {
		return fallback, nil
	}

	return nil, fmt.Errorf("no suitable IPv%d address found", protocol)
}

func isSuitableOutboundIP(ip net.IP, protocol int) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
		return false
	}

	isIPv6 := ip.To4() == nil
	if protocol == 6 {
		// Link-local addresses cannot be used as a source without a zone.
		return isIPv6 && !ip.IsLinkLocalUnicast()
	}

	return !isIPv6
}

// listInterfaceAddresses returns all addresses of one interface, or of every
// usable interface when interfaceName is empty.
func listInterfaceAddresses(interfaceName string) ([]net.IP, error) {
	if interfaceName != "" {
		iface, err := net.InterfaceByName(interfaceName)
		if err != nil {
			return nil, fmt.Errorf("failed to get interface %s: %v", interfaceName, err)
		}

		addrs, err := interfaceAddresses(*iface)
		if err != nil {
			return nil, fmt.Errorf("failed to get addresses for interface %s: %v", interfaceName, err)
		}
		return addrs, nil
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %v", err)
	}

	var result []net.IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := interfaceAddresses(iface)
		if err != nil {
			continue
		}
		result = append(result, addrs...)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no usable network interface found")
	}

	return result, nil
}

func interfaceAddresses(iface net.Interface) ([]net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	result := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			result = append(result, v.IP)
		case *net.IPAddr:
			result = append(result, v.IP)
		}
	}

	return result, nil
}

// InterfaceDetails is the UI-facing view of an interface with IPv6 addresses
// in CIDR form and a suggested rotation prefix.
type InterfaceDetails struct {
	Name            string   `json:"name"`
	Addresses       []string `json:"addresses"`
	SuggestedPrefix string   `json:"suggested_prefix"`
}

func ListInterfaceDetails() ([]InterfaceDetails, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %v", err)
	}

	result := make([]InterfaceDetails, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		detail := InterfaceDetails{Name: iface.Name, Addresses: []string{}}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() != nil || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			detail.Addresses = append(detail.Addresses, ipNet.String())
			if detail.SuggestedPrefix == "" {
				detail.SuggestedPrefix = suggestedPrefix(ipNet)
			}
		}
		if len(detail.Addresses) > 0 {
			result = append(result, detail)
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func suggestedPrefix(ipNet *net.IPNet) string {
	if ipNet == nil || ipNet.IP.To4() != nil || ipNet.IP.IsLinkLocalUnicast() {
		return ""
	}
	ones, bits := ipNet.Mask.Size()
	if ones != 64 || bits != 128 {
		return ""
	}
	return ipNet.IP.Mask(ipNet.Mask).String() + "/64"
}

func detectSubnetFromDetails(interfaceName string, details []InterfaceDetails) string {
	for _, detail := range details {
		if detail.Name == interfaceName && detail.SuggestedPrefix != "" {
			return detail.SuggestedPrefix
		}
	}
	return ""
}
