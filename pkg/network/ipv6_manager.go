package network

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// Pool is the subset of IPv6Manager used by proxy implementations and status
// reporting. It exists so tests can supply a fake without touching the host.
type Pool interface {
	GetRandomIPv6() (net.IP, error)
	IsPoolAddress(net.IP) bool
	Stop()
	GetPoolSize() int
	GetTotalRotated() int64
	Prefix() string
	Snapshot() PoolSnapshot
}

// AddressSnapshot describes one address currently assigned to the interface.
type AddressSnapshot struct {
	Address  string    `json:"address"`
	Usage    int       `json:"usage"`
	LastUsed time.Time `json:"last_used"`
	AddedAt  time.Time `json:"added_at"`
}

// PoolSnapshot is a consistent point-in-time view of the rotation pool.
type PoolSnapshot struct {
	Size          int               `json:"size"`
	Prefix        string            `json:"prefix"`
	Rotations     int64             `json:"rotations"`
	MaxUsage      int               `json:"max_usage"`
	MaxAgeSeconds int64             `json:"max_age_seconds"`
	Cursor        int               `json:"cursor"`
	TotalUsage    int64             `json:"total_usage"`
	LastRotatedAt *time.Time        `json:"last_rotated_at,omitempty"`
	LastCheckedAt *time.Time        `json:"last_checked_at,omitempty"`
	Addresses     []AddressSnapshot `json:"addresses"`
}

// IPStats tracks usage statistics for each IP in the pool
type IPStats struct {
	usageCount int       // How many times this IP was used
	lastUsed   time.Time // When was it last used
	addedAt    time.Time // When was it added to the pool
}

// IPv6Manager manages dynamic IPv6 address rotation with pre-populated IP pool
type IPv6Manager struct {
	mu            sync.Mutex
	interfaceName string
	subnet        *net.IPNet
	ipPool        []net.IP            // Pre-populated pool of IPs
	ipStats       map[string]*IPStats // Usage statistics per IP
	poolIndex     int                 // Current position in the pool
	maxUsageCount int                 // Max times an IP can be used before rotation
	maxAge        time.Duration       // Max age of an IP before rotation
	rotationStop  chan bool           // Channel to stop the rotation goroutine
	stopOnce      sync.Once           // Ensures cleanup runs at most once
	totalRotated  int64               // Total number of IPs rotated
	totalUsage    int64               // Total number of address selections
	lastRotatedAt time.Time           // Last time at least one address was rotated
	lastCheckedAt time.Time           // Last time the rotation pass ran
	runner        CommandRunner
	poolSize      int
}

// NewIPv6Manager creates a new IPv6 manager with configurable rotation settings
func NewIPv6Manager(interfaceName string, subnet string, poolSize, maxUsage, maxAgeMinutes int) (*IPv6Manager, error) {
	return NewIPv6ManagerWithRunner(interfaceName, subnet, poolSize, maxUsage, maxAgeMinutes, execRunner{})
}

// NewIPv6ManagerWithRunner creates an IPv6 manager that executes system
// commands through runner.
func NewIPv6ManagerWithRunner(interfaceName string, subnet string, poolSize, maxUsage, maxAgeMinutes int, runner CommandRunner) (*IPv6Manager, error) {
	if runner == nil {
		runner = execRunner{}
	}

	if interfaceName == "" {
		log.Printf("WARNING: no interface specified, attempting auto-detection")
		iface, err := getDefaultInterface()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-detect network interface: %v; set network.interface_name", err)
		}
		interfaceName = iface.Name
		log.Printf("auto-detected interface: %s", interfaceName)
	}

	if subnet == "" {
		detected, err := detectRotationPrefixWithRunner(runner, interfaceName)
		if err != nil {
			return nil, fmt.Errorf("failed to auto-detect IPv6 subnet: %v; set network.ipv6_subnet", err)
		}
		subnet = detected
		log.Printf("auto-detected IPv6 subnet: %s", subnet)
	}

	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %v", err)
	}

	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("network interface %q not found: %v", interfaceName, err)
	}
	if iface.Flags&net.FlagUp == 0 {
		return nil, fmt.Errorf("network interface %q is down", interfaceName)
	}

	if poolSize <= 0 {
		poolSize = 200
	}
	if maxUsage <= 0 {
		maxUsage = 100
	}
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 30
	}

	mgr := &IPv6Manager{
		interfaceName: interfaceName,
		subnet:        ipnet,
		ipPool:        make([]net.IP, 0),
		ipStats:       make(map[string]*IPStats),
		maxUsageCount: maxUsage,
		maxAge:        time.Duration(maxAgeMinutes) * time.Minute,
		rotationStop:  make(chan bool),
		runner:        runner,
		poolSize:      poolSize,
	}

	WarnIfNdppdNotRunning()

	log.Printf("IPv6 rotation mode: interface=%s subnet=%s pool=%d max_usage=%d max_age=%v",
		interfaceName, ipnet.String(), poolSize, maxUsage, mgr.maxAge)

	if err := mgr.populateIPPool(poolSize); err != nil {
		mgr.Stop()
		return nil, fmt.Errorf("failed to populate IP pool: %v", err)
	}

	log.Printf("IP pool ready with %d addresses", poolSize)
	go mgr.rotateOldIPs()
	return mgr, nil
}

// GetRandomIPv6 returns a random IPv6 address from the pre-populated pool
// This is fast because all IPs are already added to the interface
func (m *IPv6Manager) GetRandomIPv6() (net.IP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.ipPool) == 0 {
		return nil, fmt.Errorf("IP pool is empty")
	}

	// Round-robin through the pool for even distribution
	ip := m.ipPool[m.poolIndex]
	m.poolIndex = (m.poolIndex + 1) % len(m.ipPool)

	// Update usage statistics
	ipStr := ip.String()
	if stats, exists := m.ipStats[ipStr]; exists {
		stats.usageCount++
		stats.lastUsed = time.Now()
	} else {
		// This shouldn't happen, but handle it gracefully
		m.ipStats[ipStr] = &IPStats{
			usageCount: 1,
			lastUsed:   time.Now(),
			addedAt:    time.Now(),
		}
	}
	m.totalUsage++

	return ip, nil
}

// populateIPPool generates and adds a pool of IPs to the interface
// This is done once at startup, so subsequent requests are fast
func (m *IPv6Manager) populateIPPool(size int) error {
	log.Printf("Adding %d IPv6 addresses to interface (this may take 10-30 seconds)...", size)

	for i := 0; i < size; i++ {
		ip := m.generateRandomIPv6()

		// Add IP to interface
		if err := m.assignIPToInterface(ip); err != nil {
			// Ignore "already exists" errors
			errStr := err.Error()
			if !strings.Contains(errStr, "File exists") && !strings.Contains(errStr, "RTNETLINK answers") {
				return fmt.Errorf("failed to add IP %s: %v", ip.String(), err)
			}
		}

		m.ipPool = append(m.ipPool, ip)

		// Initialize stats for this IP
		m.ipStats[ip.String()] = &IPStats{
			usageCount: 0,
			lastUsed:   time.Now(),
			addedAt:    time.Now(),
		}

		// Progress indicator every 50 IPs
		if (i+1)%50 == 0 {
			log.Printf("Progress: %d/%d IPs added", i+1, size)
		}
	}

	return nil
}

// rotateOldIPs runs in background and replaces old/overused IPs with fresh ones
func (m *IPv6Manager) rotateOldIPs() {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAndRotateIPs()
		case <-m.rotationStop:
			return
		}
	}
}

// checkAndRotateIPs checks all IPs and replaces old/overused ones
func (m *IPv6Manager) checkAndRotateIPs() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	rotatedCount := 0

	for i, ip := range m.ipPool {
		ipStr := ip.String()
		stats, exists := m.ipStats[ipStr]

		if !exists {
			continue
		}

		// Check if IP should be rotated
		shouldRotate := false

		if stats.usageCount >= m.maxUsageCount {
			shouldRotate = true
		} else if now.Sub(stats.addedAt) >= m.maxAge {
			shouldRotate = true
		}

		if shouldRotate {
			// Generate new IP
			newIP := m.generateRandomIPv6()

			// Remove old IP from interface
			m.removeIPFromInterface(ipStr)

			// Add new IP to interface
			if err := m.assignIPToInterface(newIP); err != nil {
				errStr := err.Error()
				if !strings.Contains(errStr, "File exists") && !strings.Contains(errStr, "RTNETLINK answers") {
					log.Printf("warning: failed to add new IP during rotation: %v", err)
					continue
				}
			}

			// Update pool and stats
			m.ipPool[i] = newIP
			delete(m.ipStats, ipStr)
			m.ipStats[newIP.String()] = &IPStats{
				usageCount: 0,
				lastUsed:   now,
				addedAt:    now,
			}

			rotatedCount++
		}
	}

	if rotatedCount > 0 {
		m.totalRotated += int64(rotatedCount)
		m.lastRotatedAt = now
		log.Printf("Rotated %d IPs (%s)", rotatedCount, time.Now().Format("15:04:05"))
	}
	m.lastCheckedAt = now
}

// generateRandomIPv6 generates a random IPv6 address within the subnet
func (m *IPv6Manager) generateRandomIPv6() net.IP {
	// Start with the subnet base
	ip := make(net.IP, len(m.subnet.IP))
	copy(ip, m.subnet.IP)

	// For a /64 subnet, randomize the last 64 bits (bytes 8-15)
	for i := 8; i < 16; i++ {
		ip[i] = byte(rand.Intn(256))
	}

	// Avoid special addresses
	if ip[15] == 0 || ip[15] == 1 {
		ip[15] = byte(rand.Intn(254) + 2)
	}

	return ip
}

// assignIPToInterface adds the IP address to the network interface
func (m *IPv6Manager) assignIPToInterface(ip net.IP) error {
	output, err := m.runner.Run("ip", "-6", "addr", "add", fmt.Sprintf("%s/128", ip.String()), "dev", m.interfaceName)
	if err != nil {
		outputStr := string(output)
		// Check if error is because IP already exists - this is OK
		if strings.Contains(outputStr, "File exists") || strings.Contains(outputStr, "RTNETLINK answers: File exists") {
			return nil
		}
		// For any other error, return it
		if len(outputStr) > 0 {
			return fmt.Errorf("failed to add IP %s: %v, output: %s", ip.String(), err, outputStr)
		}
		return fmt.Errorf("failed to add IP %s: %v", ip.String(), err)
	}
	return nil
}

// removeIPFromInterface removes the IP address from the network interface
func (m *IPv6Manager) removeIPFromInterface(ip string) error {
	output, err := m.runner.Run("ip", "-6", "addr", "del", fmt.Sprintf("%s/128", ip), "dev", m.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to remove IP %s: %v, output: %s", ip, err, string(output))
	}
	return nil
}

// Prefix returns the manager's subnet in CIDR form.
func (m *IPv6Manager) Prefix() string {
	if m.subnet == nil {
		return ""
	}
	return m.subnet.String()
}

// IsPoolAddress reports whether ip is currently part of the rotation pool.
func (m *IPv6Manager) IsPoolAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.ipStats[ip.String()]
	return exists
}

// Stop stops the IPv6 manager and cleans up the IP pool.
// It is safe to call Stop more than once.
func (m *IPv6Manager) Stop() {
	m.stopOnce.Do(func() {
		if m.rotationStop != nil {
			close(m.rotationStop)
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		log.Printf("Cleaning up IP pool (%d addresses)...", len(m.ipPool))

		// Remove all IPs from the interface
		for _, ip := range m.ipPool {
			if err := m.removeIPFromInterface(ip.String()); err != nil {
				log.Printf("warning: failed to remove IP %s during cleanup: %v", ip, err)
			}
		}

		m.ipPool = nil
		m.ipStats = nil
		log.Printf("IP pool cleaned up")
	})
}

// GetPoolSize returns the current size of the IP pool
func (m *IPv6Manager) GetPoolSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ipPool)
}

// GetTotalRotated returns the total number of IPs that have been rotated
func (m *IPv6Manager) GetTotalRotated() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalRotated
}

// Snapshot returns a consistent view of the pool for status reporting.
func (m *IPv6Manager) Snapshot() PoolSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap := PoolSnapshot{
		Size:          len(m.ipPool),
		MaxUsage:      m.maxUsageCount,
		MaxAgeSeconds: int64(m.maxAge / time.Second),
		Cursor:        m.poolIndex,
		Rotations:     m.totalRotated,
		TotalUsage:    m.totalUsage,
		Addresses:     make([]AddressSnapshot, 0, len(m.ipPool)),
	}
	if m.subnet != nil {
		snap.Prefix = m.subnet.String()
	}
	if !m.lastRotatedAt.IsZero() {
		t := m.lastRotatedAt
		snap.LastRotatedAt = &t
	}
	if !m.lastCheckedAt.IsZero() {
		t := m.lastCheckedAt
		snap.LastCheckedAt = &t
	}
	for _, ip := range m.ipPool {
		stats, ok := m.ipStats[ip.String()]
		if !ok {
			continue
		}
		snap.Addresses = append(snap.Addresses, AddressSnapshot{
			Address:  ip.String(),
			Usage:    stats.usageCount,
			LastUsed: stats.lastUsed,
			AddedAt:  stats.addedAt,
		})
	}
	return snap
}
