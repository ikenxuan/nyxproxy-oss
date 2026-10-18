package config

import "os"

var restartRequiredFields = []string{
	"web.enabled",
	"web.listen_address",
	"web.port",
	"web.tls_cert_file",
	"web.tls_key_file",
	"web.trust_proxy_headers",
	"web.cookie_secure",
}

// LockedFields reports configuration fields that cannot be changed through
// the web console. The value is either "env" for environment-managed fields
// or "restart_required" for fields that need a process restart.
func LockedFields() map[string]string {
	locked := map[string]string{}
	if os.Getenv("NYXPROXY_PREFIX") != "" {
		locked["network.ipv6_subnet"] = "env"
	}
	if os.Getenv("NYXPROXY_INTERFACE") != "" {
		locked["network.interface_name"] = "env"
	}
	for _, field := range restartRequiredFields {
		locked[field] = "restart_required"
	}
	return locked
}

// MergeUpdate merges a submitted configuration into the current one,
// rejecting changes to locked fields and preserving stored passwords when the
// submission leaves them empty.
func MergeUpdate(current, submitted ProxyConfig) (ProxyConfig, error) {
	locked := LockedFields()
	fields := map[string]string{}
	reject := func(field string, changed bool) {
		if !changed {
			return
		}
		if locked[field] == "env" {
			fields[field] = "managed by environment"
		} else {
			fields[field] = "requires restart"
		}
	}

	reject("network.ipv6_subnet", submitted.Network.IPv6Subnet != current.Network.IPv6Subnet)
	reject("network.interface_name", submitted.Network.InterfaceName != current.Network.InterfaceName)
	reject("web.enabled", submitted.Web.Enabled != current.Web.Enabled)
	reject("web.listen_address", submitted.Web.ListenAddress != current.Web.ListenAddress)
	reject("web.port", submitted.Web.Port != current.Web.Port)
	reject("web.tls_cert_file", submitted.Web.TLSCertFile != current.Web.TLSCertFile)
	reject("web.tls_key_file", submitted.Web.TLSKeyFile != current.Web.TLSKeyFile)
	reject("web.trust_proxy_headers", submitted.Web.TrustProxyHeaders != current.Web.TrustProxyHeaders)
	reject("web.cookie_secure", submitted.Web.CookieSecure != current.Web.CookieSecure)

	if len(fields) > 0 {
		return ProxyConfig{}, &ValidationError{Fields: fields}
	}

	merged := submitted
	if locked["network.ipv6_subnet"] == "env" {
		merged.Network.IPv6Subnet = current.Network.IPv6Subnet
	}
	if locked["network.interface_name"] == "env" {
		merged.Network.InterfaceName = current.Network.InterfaceName
	}
	merged.Web.Enabled = current.Web.Enabled
	merged.Web.ListenAddress = current.Web.ListenAddress
	merged.Web.Port = current.Web.Port
	merged.Web.TLSCertFile = current.Web.TLSCertFile
	merged.Web.TLSKeyFile = current.Web.TLSKeyFile
	merged.Web.TrustProxyHeaders = current.Web.TrustProxyHeaders
	merged.Web.CookieSecure = current.Web.CookieSecure

	if merged.Proxy.Password == "" {
		merged.Proxy.Password = current.Proxy.Password
	}
	if merged.Web.Password == "" {
		merged.Web.Password = current.Web.Password
	}
	return merged, nil
}

// Redacted returns a copy of the configuration with passwords removed.
func (c ProxyConfig) Redacted() ProxyConfig {
	c.Proxy.Password = ""
	c.Web.Password = ""
	return c
}

// PasswordsSet reports which password fields currently have a value.
func (c ProxyConfig) PasswordsSet() map[string]bool {
	return map[string]bool{
		"proxy.password": c.Proxy.Password != "",
		"web.password":   c.Web.Password != "",
	}
}
