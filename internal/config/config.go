package config

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProxyConfig holds the configuration for the proxy server
type ProxyConfig struct {
	// Proxy settings
	Proxy ProxySettings `yaml:"proxy" json:"proxy"`
	// Network settings
	Network NetworkSettings `yaml:"network" json:"network"`
	// Monitoring settings
	Monitoring MonitoringSettings `yaml:"monitoring" json:"monitoring"`
	// Logging settings
	Logging LoggingSettings `yaml:"logging" json:"logging"`
	// Web console settings
	Web WebSettings `yaml:"web" json:"web"`
}

// ProxySettings contains proxy-specific configuration
type ProxySettings struct {
	// Type defines the type of proxy (socks5 or https)
	Type string `yaml:"type" json:"type"`
	// ListenAddress is the address where the proxy will listen
	ListenAddress string `yaml:"listen_address" json:"listen_address"`
	// ListenPort is the port where the proxy will listen
	ListenPort int `yaml:"listen_port" json:"listen_port"`
	// Username for proxy authentication
	Username string `yaml:"username" json:"username"`
	// Password for proxy authentication
	Password string `yaml:"password" json:"password"`
}

// NetworkSettings contains network-specific configuration
type NetworkSettings struct {
	// InterfaceName is the name of the network interface to use
	InterfaceName string `yaml:"interface_name" json:"interface_name"`
	// IPv4Enabled enables IPv4 for outgoing connections
	IPv4Enabled bool `yaml:"ipv4_enabled" json:"ipv4_enabled"`
	// IPv6Enabled enables IPv6 for outgoing connections
	IPv6Enabled bool `yaml:"ipv6_enabled" json:"ipv6_enabled"`
	// IPv6Subnet is the IPv6 subnet for rotating IPs (e.g., "2a05:f480:1800:25db::/64")
	IPv6Subnet string `yaml:"ipv6_subnet" json:"ipv6_subnet"`
	// DNSSourceAddress optionally pins the source address used for DNS queries
	// (e.g., a stable SLAAC address). When empty, a non-pool address is chosen
	// automatically and re-evaluated for every query.
	DNSSourceAddress string `yaml:"dns_source_address" json:"dns_source_address"`
	// RotateIPv6 enables rotating IPv6 addresses from the subnet
	RotateIPv6 bool `yaml:"rotate_ipv6" json:"rotate_ipv6"`
	// IPv6PoolSize is the number of IPs in the rotation pool (default: 200)
	IPv6PoolSize int `yaml:"ipv6_pool_size" json:"ipv6_pool_size"`
	// IPv6MaxUsage is the max number of times an IP can be used before rotation (default: 100)
	IPv6MaxUsage int `yaml:"ipv6_max_usage" json:"ipv6_max_usage"`
	// IPv6MaxAge is the max age in minutes before an IP is rotated (default: 30)
	IPv6MaxAge int `yaml:"ipv6_max_age" json:"ipv6_max_age"`
}

// MonitoringSettings contains monitoring endpoint configuration
type MonitoringSettings struct {
	// Enabled enables the monitoring endpoint
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Port is the port for the monitoring endpoint
	Port int `yaml:"port" json:"port"`
	// AllowRemote allows remote access to monitoring (false = localhost only)
	AllowRemote bool `yaml:"allow_remote" json:"allow_remote"`
}

// LoggingSettings contains logging configuration
type LoggingSettings struct {
	// DebugLevel controls the level of debug output (0=none, 1=basic, 2=detailed)
	DebugLevel int `yaml:"debug_level" json:"debug_level"`
}

// WebSettings contains web console configuration
type WebSettings struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	ListenAddress     string `yaml:"listen_address" json:"listen_address"`
	Port              int    `yaml:"port" json:"port"`
	Username          string `yaml:"username" json:"username"`
	Password          string `yaml:"password" json:"password"`
	TLSCertFile       string `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile        string `yaml:"tls_key_file" json:"tls_key_file"`
	TrustProxyHeaders bool   `yaml:"trust_proxy_headers" json:"trust_proxy_headers"`
	CookieSecure      bool   `yaml:"cookie_secure" json:"cookie_secure"`
}

// Legacy fields for backward compatibility
func (c *ProxyConfig) GetListenAddress() string {
	return c.Proxy.ListenAddress
}

func (c *ProxyConfig) GetListenPort() int {
	return c.Proxy.ListenPort
}

func (c *ProxyConfig) GetProxyType() string {
	return c.Proxy.Type
}

func (c *ProxyConfig) GetUsername() string {
	return c.Proxy.Username
}

func (c *ProxyConfig) GetPassword() string {
	return c.Proxy.Password
}

func (c *ProxyConfig) GetDebugLevel() int {
	return c.Logging.DebugLevel
}

func (c *ProxyConfig) GetProxyProtocol() int {
	// Determine protocol based on enabled flags
	if c.Network.IPv6Enabled && !c.Network.IPv4Enabled {
		return 6
	}
	return 4 // Default to IPv4
}

// LoadConfig loads the configuration from config.yaml or environment variables
func LoadConfig() (*ProxyConfig, error) {
	return LoadConfigFrom("config.yaml")
}

// LoadConfigFrom loads the configuration from the given YAML file, falling
// back to environment variables when the file does not exist.
func LoadConfigFrom(path string) (*ProxyConfig, error) {
	cfg := &ProxyConfig{}

	if _, err := os.Stat(path); err == nil {
		if err := loadFromYAML(path, cfg); err != nil {
			return nil, fmt.Errorf("failed to load %s: %v", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat %s: %v", path, err)
	} else {
		loadFromEnv(cfg)
	}

	applyDefaults(cfg)
	ApplyRuntimeOverrides(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyDefaults(cfg *ProxyConfig) {
	if cfg.Web.ListenAddress == "" {
		cfg.Web.ListenAddress = "127.0.0.1"
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 18081
	}
	if cfg.Web.Username == "" {
		cfg.Web.Username = "admin"
	}
}

// ApplyRuntimeOverrides overrides configuration values managed by the
// runtime environment.
func ApplyRuntimeOverrides(cfg *ProxyConfig) {
	if v := os.Getenv("NYXPROXY_PREFIX"); v != "" {
		cfg.Network.IPv6Subnet = v
	}
	if v := os.Getenv("NYXPROXY_INTERFACE"); v != "" {
		cfg.Network.InterfaceName = v
	}
}

// Validate applies defaults and runtime overrides, then validates the
// configuration.
func (c *ProxyConfig) Validate() error {
	applyDefaults(c)
	ApplyRuntimeOverrides(c)
	return validateConfig(c)
}

// loadFromYAML loads configuration from a YAML file
func loadFromYAML(filename string, cfg *ProxyConfig) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return err
	}

	return nil
}

// loadFromEnv loads configuration from environment variables (backward compatibility)
func loadFromEnv(cfg *ProxyConfig) {
	cfg.Proxy.ListenAddress = getEnvString("PROXY_LISTEN_ADDRESS", "0.0.0.0")
	cfg.Proxy.ListenPort = getEnvInt("PROXY_LISTEN_PORT", 8080)
	cfg.Proxy.Type = getEnvString("PROXY_TYPE", "https")
	cfg.Proxy.Username = getEnvString("PROXY_USERNAME", "admin")
	cfg.Proxy.Password = getEnvString("PROXY_PASSWORD", "")

	protocol := getEnvInt("PROXY_PROTOCOL", 4)
	cfg.Network.InterfaceName = getEnvString("NETWORK_INTERFACE", "")
	cfg.Network.DNSSourceAddress = getEnvString("PROXY_DNS_SOURCE_ADDRESS", "")
	cfg.Network.IPv4Enabled = protocol == 4
	cfg.Network.IPv6Enabled = protocol == 6

	cfg.Monitoring.Enabled = getEnvBool("MONITORING_ENABLED", true)
	cfg.Monitoring.Port = getEnvInt("MONITORING_PORT", 9090)
	cfg.Monitoring.AllowRemote = getEnvBool("MONITORING_ALLOW_REMOTE", false)

	cfg.Logging.DebugLevel = getEnvInt("DEBUG_LEVEL", 0)
}

// ValidationError reports configuration problems keyed by dotted field name.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+e.Fields[k])
	}
	return strings.Join(parts, "; ")
}

var lookPath = exec.LookPath

// validateConfig validates the configuration and returns a ValidationError
// describing every invalid field.
func validateConfig(cfg *ProxyConfig) error {
	fields := map[string]string{}
	add := func(field, msg string) {
		if _, exists := fields[field]; !exists {
			fields[field] = msg
		}
	}

	if cfg.Proxy.Type != "https" && cfg.Proxy.Type != "socks5" {
		add("proxy.type", "must be 'https' or 'socks5'")
	}
	if cfg.Proxy.Username == "" {
		add("proxy.username", "must be set")
	}
	if cfg.Proxy.Password == "" {
		add("proxy.password", "must be set")
	}
	if !cfg.Network.IPv4Enabled && !cfg.Network.IPv6Enabled {
		add("network.ipv4_enabled", "at least one of ipv4_enabled or ipv6_enabled must be true")
	}

	if cfg.Network.DNSSourceAddress != "" {
		ip := net.ParseIP(cfg.Network.DNSSourceAddress)
		if ip == nil {
			add("network.dns_source_address", "must be a valid IP")
		} else if ip.IsLinkLocalUnicast() {
			add("network.dns_source_address", "must not be link-local")
		} else if wantsIPv6 := cfg.GetProxyProtocol() == 6; (ip.To4() == nil) != wantsIPv6 {
			add("network.dns_source_address", fmt.Sprintf("must match IPv%d mode", cfg.GetProxyProtocol()))
		}
	}

	validPort := func(field string, port int) bool {
		if port < 1 || port > 65535 {
			add(field, "must be between 1 and 65535")
			return false
		}
		return true
	}
	proxyPortOK := validPort("proxy.listen_port", cfg.Proxy.ListenPort)
	monitorPortOK := !cfg.Monitoring.Enabled || validPort("monitoring.port", cfg.Monitoring.Port)
	webPortOK := !cfg.Web.Enabled || validPort("web.port", cfg.Web.Port)

	if cfg.Network.IPv6Subnet != "" && !isValidIPv6Subnet(cfg.Network.IPv6Subnet) {
		add("network.ipv6_subnet", "must be a valid IPv6 /64")
	}
	if cfg.Network.RotateIPv6 {
		if cfg.Network.IPv6PoolSize <= 0 {
			add("network.ipv6_pool_size", "must be greater than 0")
		}
		if cfg.Network.IPv6MaxUsage <= 0 {
			add("network.ipv6_max_usage", "must be greater than 0")
		}
		if cfg.Network.IPv6MaxAge <= 0 {
			add("network.ipv6_max_age", "must be greater than 0")
		}
		if _, err := lookPath("ip"); err != nil {
			add("network.rotate_ipv6", "the ip command is required for IPv6 rotation")
		}
	}

	if cfg.Web.Enabled {
		if cfg.Web.Username == "" {
			add("web.username", "must be set")
		}
		if cfg.Web.Password == "" {
			add("web.password", "must be set")
		}
		if cfg.Web.ListenAddress != "" && cfg.Web.ListenAddress != "localhost" && net.ParseIP(cfg.Web.ListenAddress) == nil {
			add("web.listen_address", "must be a valid IP or localhost")
		}
		if (cfg.Web.TLSCertFile == "") != (cfg.Web.TLSKeyFile == "") {
			add("web.tls_cert_file", "tls_cert_file and tls_key_file must both be set or both empty")
		}
		for field, path := range map[string]string{
			"web.tls_cert_file": cfg.Web.TLSCertFile,
			"web.tls_key_file":  cfg.Web.TLSKeyFile,
		} {
			if path == "" {
				continue
			}
			f, err := os.Open(path)
			if err != nil {
				add(field, "must be readable")
				continue
			}
			f.Close()
		}
	}

	monitorAddr := "127.0.0.1"
	if cfg.Monitoring.AllowRemote {
		monitorAddr = "0.0.0.0"
	}
	if proxyPortOK && monitorPortOK && cfg.Monitoring.Enabled &&
		listenersConflict(cfg.Proxy.ListenAddress, cfg.Proxy.ListenPort, monitorAddr, cfg.Monitoring.Port) {
		add("monitoring.port", "conflicts with proxy.listen_port")
	}
	if proxyPortOK && webPortOK && cfg.Web.Enabled &&
		listenersConflict(cfg.Proxy.ListenAddress, cfg.Proxy.ListenPort, cfg.Web.ListenAddress, cfg.Web.Port) {
		add("web.port", "conflicts with proxy.listen_port")
	}
	if cfg.Monitoring.Enabled && webPortOK && monitorPortOK &&
		listenersConflict(monitorAddr, cfg.Monitoring.Port, cfg.Web.ListenAddress, cfg.Web.Port) {
		add("web.port", "conflicts with monitoring.port")
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func isValidIPv6Subnet(s string) bool {
	_, n, err := net.ParseCIDR(s)
	if err != nil || n.IP.To4() != nil {
		return false
	}
	ones, bits := n.Mask.Size()
	return ones == 64 && bits == 128
}

func listenersConflict(aAddr string, aPort int, bAddr string, bPort int) bool {
	if aPort != bPort {
		return false
	}
	if aAddr == "" {
		aAddr = "0.0.0.0"
	}
	if bAddr == "" {
		bAddr = "0.0.0.0"
	}
	if aAddr == bAddr {
		return true
	}
	return aAddr == "0.0.0.0" || aAddr == "::" || bAddr == "0.0.0.0" || bAddr == "::"
}

// Helper functions for environment variables
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
