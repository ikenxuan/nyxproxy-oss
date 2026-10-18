# NyxProxy-OSS Web Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a built-in web console to NyxProxy-OSS for Linux and Docker: login, live status, configuration editing with transactional auto-apply/rollback, and an SSE log viewer, while keeping a single Go binary and a single container.

**Architecture:** A Go supervisor owns the proxy and monitoring components and restarts them on configuration changes, while the API server stays online. The Next.js frontend is statically exported, embedded with `go:embed`, and served by the Go API server on the same origin. Configuration is persisted to a YAML file atomically; failed applies roll back to the last good configuration.

**Tech Stack:** Go 1.27, `net/http`, YAML v3, Next.js App Router, TypeScript, Tailwind CSS, pnpm 12.9.1, Vitest + React Testing Library, Docker multi-stage builds.

**Spec:** `docs/superpowers/specs/2026-10-18-web-console-design.md`

## Global Constraints

- Platform: Linux amd64/arm64 and Docker only. Remove Windows build targets and Windows documentation.
- Go: 1.27 in `go.mod`, Dockerfile, and CI.
- Frontend: Node 24 LTS, pnpm 12.9.1 via `packageManager`; commit `web/pnpm-lock.yaml`. No other package manager anywhere.
- Frontend output: Next.js static export embedded with `go:embed`; no Node runtime in the final binary or container.
- Web console defaults: disabled when the `web` section is absent; Docker template enables it on `0.0.0.0:18081`; bare-metal default is `127.0.0.1:18081`.
- Sessions: 12-hour absolute expiry, at most 100 sessions, HttpOnly + SameSite=Strict cookie, CSRF token required for mutations.
- Login rate limit: 5 failures per IP per 5 minutes, 429 with `Retry-After`, 200ms delay per failure.
- Apply drain timeout: 5 seconds. Address pool is cleaned before the new proxy starts.
- SSE: 2000-line ring buffer, 8 concurrent streams, 256-entry subscriber buffer, 15-second heartbeat, 30-minute connection lifetime.
- Config file permissions: 0600; `/data` directory 0700.
- Test commands: `go test -race ./...`, `go vet ./...`, `pnpm --dir web lint`, `pnpm --dir web test`, `pnpm --dir web build`.
- Docker is not installed in the current Windows environment; Docker image checks run in CI or on a Linux host.
- Commit policy: commit steps below are checkpoints. Execute them only if the user has authorized commits. Never mix the existing uncommitted Docker/DNS work into this feature's commits.
- Do not mention other proxy projects in READMEs or user-facing documentation.

---

## File Structure

**Backend**

- `internal/config/` - schema, defaults, validation, runtime overrides, password redaction, merge, atomic `Store` (`config.go`, `merge.go`, `store.go`).
- `pkg/network/` - injectable command runner, pool interface, address add/remove, interface discovery (`runner.go`, `ipv6_manager.go`, `interfaces.go`).
- `pkg/proxy/` - shared lifecycle types (`State`, `ProxyStatus`, `Proxy`).
- `pkg/socks5/`, `pkg/https/`, `pkg/monitoring/` - listener ownership, `Start`/`Wait`/`Stop`/`Status`, drain.
- `internal/logbuf/` - log broker, ring buffer, SSE subscription/replay.
- `internal/app/` - supervisor, apply/rollback transaction, status aggregation, error types.
- `internal/api/` - HTTP server, auth/session/CSRF, handlers, security middleware.
- `internal/webui/` - embedded static export (`embed.go`, generated `dist/`).
- `cmd/proxy/` - wiring, config path precedence, startup/shutdown, healthcheck mode.

**Frontend** (`web/`)

- `app/` - layouts, providers, login, dashboard, config, logs pages.
- `components/` - app shell, login, config form/field, UI primitives.
- `lib/` - API client, types, i18n, theme, auth, dirty state, log store, diff/format helpers.

**Build and deployment**

- `scripts/build-web.sh` - pnpm install/build and copy export into `internal/webui/dist`.
- `scripts/smoke-test.sh` - local binary acceptance smoke test.
- `Makefile` - `web`, `build`, `test`, `lint`, `clean`.
- `Dockerfile`, `docker/entrypoint.sh`, `docker/config.template.yaml`, `docker-compose.yml`, `docker/docker-compose.example.yml`, `.dockerignore`, `.env.docker.example`.
- `.github/workflows/` - CI, release, Docker publish.
- `README.md`, `README-ZH.md`, `docs/CONFIG.md`, `config.example.yaml`, `scripts/quick-setup.sh` - user documentation and setup.

---

## Preflight (once)

- [ ] **Step 1: Make Go available in the shell**

PowerShell:

```powershell
$env:PATH = "C:\Program Files\Go\bin;$env:PATH"
go version
```

Expected: `go version go1.27... windows/amd64` (or a Linux Go 1.27 toolchain when running in CI).

- [ ] **Step 2: Inspect the working tree**

```bash
git status --short
```

The tree currently contains uncommitted Docker/DNS changes. Decide with the user whether to commit them as a separate baseline or stash them. Do not fold them into this feature's commits.

- [ ] **Step 3: Run the baseline tests**

```bash
go test ./...
```

Expected: all existing packages pass. Record any pre-existing failures; do not fix unrelated failures in this plan.

- [ ] **Step 4: Create the feature branch**

```bash
git switch -c feat/web-console
```

If the user has not authorized a branch/commits, skip this step and keep changes in the working tree.

- [ ] **Step 5: Verify frontend tooling**

```powershell
node -v
pnpm --version
```

Expected: Node `v24.x`, pnpm `12.9.1`. If pnpm differs, run `corepack prepare pnpm@12.9.1 --activate`.

---

## Task 1: Configuration Schema, Merge, and Atomic Store

**Files:**
- Modify: `go.mod`
- Modify: `internal/config/config.go`
- Create: `internal/config/merge.go`
- Create: `internal/config/store.go`
- Create: `internal/config/merge_test.go`
- Create: `internal/config/store_test.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type WebSettings struct { Enabled bool; ListenAddress string; Port int; Username string; Password string; TLSCertFile string; TLSKeyFile string; TrustProxyHeaders bool; CookieSecure bool }`
  - `func LoadConfigFrom(path string) (*ProxyConfig, error)`
  - `func (c *ProxyConfig) Validate() error`
  - `func ApplyRuntimeOverrides(cfg *ProxyConfig)`
  - `func LockedFields() map[string]string`
  - `func MergeUpdate(current, submitted ProxyConfig) (ProxyConfig, error)`
  - `func (c ProxyConfig) Redacted() ProxyConfig`
  - `func (c ProxyConfig) PasswordsSet() map[string]bool`
  - `type Store struct`
  - `func NewStore(path string) (*Store, error)`
  - `func (s *Store) Snapshot() (ProxyConfig, uint64)`
  - `func (s *Store) Prepare(cfg ProxyConfig) (tmpPath string, err error)`
  - `func (s *Store) Commit(tmpPath string, cfg ProxyConfig) error`
  - `func (s *Store) Discard(tmpPath string)`
  - `func (s *Store) Path() string`
  - `type ValidationError struct { Fields map[string]string }`

- [ ] **Step 1: Write failing tests for the web schema and validation**

Create `internal/config/merge_test.go`:

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromAppliesWebDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: "0.0.0.0"
  listen_port: 1080
  username: admin
  password: secret
network:
  ipv4_enabled: true
  ipv6_enabled: false
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Web.Enabled {
		t.Fatal("web console must be disabled when the web section is absent")
	}
	if cfg.Web.ListenAddress != "127.0.0.1" {
		t.Fatalf("listen address = %q, want 127.0.0.1", cfg.Web.ListenAddress)
	}
	if cfg.Web.Port != 18081 {
		t.Fatalf("port = %d, want 18081", cfg.Web.Port)
	}
	if cfg.Web.Username != "admin" {
		t.Fatalf("username = %q, want admin", cfg.Web.Username)
	}
}

func TestValidateWebRequiresCredentials(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "127.0.0.1"
	cfg.Web.Port = 18081
	cfg.Web.Username = "admin"

	err := cfg.Validate()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["web.password"] == "" {
		t.Fatalf("missing web.password field error: %#v", verr.Fields)
	}
}

func TestValidateRejectsListenerConflict(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "0.0.0.0"
	cfg.Web.Port = cfg.Proxy.ListenPort
	cfg.Web.Username = "admin"
	cfg.Web.Password = "secret"

	err := cfg.Validate()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["web.port"] == "" {
		t.Fatalf("missing web.port conflict: %#v", verr.Fields)
	}
}

func TestRuntimeOverridesWin(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	t.Setenv("NYXPROXY_INTERFACE", "eth9")

	cfg := validBaseConfig()
	cfg.Network.IPv6Subnet = "2001:db8:1::/64"
	cfg.Network.InterfaceName = "eth0"
	ApplyRuntimeOverrides(cfg)

	if cfg.Network.IPv6Subnet != "2001:db8:ffff::/64" {
		t.Fatalf("subnet = %q", cfg.Network.IPv6Subnet)
	}
	if cfg.Network.InterfaceName != "eth9" {
		t.Fatalf("interface = %q", cfg.Network.InterfaceName)
	}
	locked := LockedFields()
	if locked["network.ipv6_subnet"] != "env" || locked["network.interface_name"] != "env" {
		t.Fatalf("locked fields = %#v", locked)
	}
}

func TestMergeUpdateRejectsLockedFieldChange(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	current := *validBaseConfig()
	current.Network.IPv6Subnet = "2001:db8:ffff::/64"
	submitted := current
	submitted.Network.IPv6Subnet = "2001:db8:eeee::/64"

	_, err := MergeUpdate(current, submitted)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if verr.Fields["network.ipv6_subnet"] == "" {
		t.Fatalf("missing locked field error: %#v", verr.Fields)
	}
}

func TestMergeUpdatePreservesPasswords(t *testing.T) {
	current := *validBaseConfig()
	current.Proxy.Password = "old-proxy"
	current.Web.Enabled = true
	current.Web.Username = "admin"
	current.Web.Password = "old-web"

	submitted := current
	submitted.Proxy.Password = ""
	submitted.Web.Password = ""

	merged, err := MergeUpdate(current, submitted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Proxy.Password != "old-proxy" {
		t.Fatalf("proxy password = %q", merged.Proxy.Password)
	}
	if merged.Web.Password != "old-web" {
		t.Fatalf("web password = %q", merged.Web.Password)
	}
}

func TestRedactedConfigHidesPasswords(t *testing.T) {
	cfg := *validBaseConfig()
	cfg.Web.Password = "web-secret"
	red := cfg.Redacted()
	if red.Proxy.Password != "" || red.Web.Password != "" {
		t.Fatalf("passwords were not redacted: %#v", red)
	}
	set := cfg.PasswordsSet()
	if !set["proxy.password"] || !set["web.password"] {
		t.Fatalf("passwords_set = %#v", set)
	}
}
```

Create `internal/config/store_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeBaseConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: "127.0.0.1"
  listen_port: 1080
  username: admin
  password: secret
network:
  ipv4_enabled: true
  ipv6_enabled: false
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStorePrepareCommitUpdatesGeneration(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	cfg, gen := store.Snapshot()
	if gen != 1 {
		t.Fatalf("generation = %d, want 1", gen)
	}
	cfg.Proxy.ListenPort = 1081

	tmp, err := store.Prepare(cfg)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat temp: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("temp mode = %o, want 0600", info.Mode().Perm())
	}
	if err := store.Commit(tmp, cfg); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, gen := store.Snapshot()
	if gen != 2 || got.Proxy.ListenPort != 1081 {
		t.Fatalf("snapshot = %+v gen=%d", got.Proxy, gen)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "listen_port: 1081") {
		t.Fatalf("persisted config missing new port:\n%s", data)
	}
}

func TestStoreDoesNotPersistRuntimeOverrides(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	t.Setenv("NYXPROXY_INTERFACE", "eth9")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: 127.0.0.1
  listen_port: 1080
  username: admin
  password: secret
network:
  interface_name: eth0
  ipv4_enabled: true
  ipv6_enabled: false
  ipv6_subnet: 2001:db8:1::/64
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	effective, _ := store.Snapshot()
	if effective.Network.IPv6Subnet != "2001:db8:ffff::/64" || effective.Network.InterfaceName != "eth9" {
		t.Fatalf("effective config = %+v", effective.Network)
	}

	tmp, err := store.Prepare(effective)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := store.Commit(tmp, effective); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved ProxyConfig
	if err := yaml.Unmarshal(persisted, &saved); err != nil {
		t.Fatalf("parse persisted config: %v", err)
	}
	if saved.Network.IPv6Subnet != "2001:db8:1::/64" {
		t.Fatalf("persisted subnet = %q, want the file value 2001:db8:1::/64", saved.Network.IPv6Subnet)
	}
	if saved.Network.InterfaceName != "eth0" {
		t.Fatalf("persisted interface = %q, want eth0", saved.Network.InterfaceName)
	}
	if strings.Contains(string(persisted), "2001:db8:ffff::/64") || strings.Contains(string(persisted), "eth9") {
		t.Fatalf("runtime overrides leaked into the file:\n%s", persisted)
	}
}

func TestStoreDiscardRemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	cfg, _ := store.Snapshot()

	tmp, err := store.Prepare(cfg)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	store.Discard(tmp)
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists: %v", err)
	}
}

func TestStoreCleansStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeBaseConfig(t, dir)
	stale := filepath.Join(dir, "config.yaml.tmp-123456")
	if err := os.WriteFile(stale, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStore(path); err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp file was not removed: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./internal/config/... -run 'Web|Runtime|Merge|Redacted|Store' -count=1
```

Expected: compile errors for `WebSettings`, `LoadConfigFrom`, `MergeUpdate`, `Store`, and related symbols.

- [ ] **Step 3: Implement the schema and validation**

In `internal/config/config.go`:

Add imports: `os/exec`, `sort`, `strings`, `time` is not needed here.

Add the `Web` field to `ProxyConfig` and add `json` tags mirroring every existing `yaml` tag:

```go
type ProxyConfig struct {
	Proxy      ProxySettings      `yaml:"proxy" json:"proxy"`
	Network    NetworkSettings    `yaml:"network" json:"network"`
	Monitoring MonitoringSettings `yaml:"monitoring" json:"monitoring"`
	Logging    LoggingSettings    `yaml:"logging" json:"logging"`
	Web        WebSettings        `yaml:"web" json:"web"`
}

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
```

Add JSON tags to `ProxySettings`, `NetworkSettings`, `MonitoringSettings`, and `LoggingSettings` using the same names as their YAML tags.

Replace `LoadConfig` with:

```go
func LoadConfig() (*ProxyConfig, error) {
	return LoadConfigFrom("config.yaml")
}

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

func ApplyRuntimeOverrides(cfg *ProxyConfig) {
	if v := os.Getenv("NYXPROXY_PREFIX"); v != "" {
		cfg.Network.IPv6Subnet = v
	}
	if v := os.Getenv("NYXPROXY_INTERFACE"); v != "" {
		cfg.Network.InterfaceName = v
	}
}

func (c *ProxyConfig) Validate() error {
	applyDefaults(c)
	ApplyRuntimeOverrides(c)
	return validateConfig(c)
}
```

Add `ValidationError` and rewrite `validateConfig`. Use a package-level `lookPath` so tests can fake the `ip` command:

```go
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
```

- [ ] **Step 4: Implement merge and redaction**

Create `internal/config/merge.go`:

```go
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

func (c ProxyConfig) Redacted() ProxyConfig {
	c.Proxy.Password = ""
	c.Web.Password = ""
	return c
}

func (c ProxyConfig) PasswordsSet() map[string]bool {
	return map[string]bool{
		"proxy.password": c.Proxy.Password != "",
		"web.password":   c.Web.Password != "",
	}
}
```

- [ ] **Step 5: Implement the atomic store**

Create `internal/config/store.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

type Store struct {
	path       string
	mu         sync.RWMutex
	cfg        ProxyConfig // effective config, including runtime overrides
	fileCfg    ProxyConfig // last persisted file view, without runtime overrides
	generation uint64
}

func NewStore(path string) (*Store, error) {
	cfg, err := LoadConfigFrom(path)
	if err != nil {
		return nil, err
	}
	fileCfg, err := loadFileConfig(path)
	if err != nil {
		return nil, err
	}
	cleanupStaleTemps(path)
	return &Store{path: path, cfg: *cfg, fileCfg: *fileCfg, generation: 1}, nil
}

// loadFileConfig parses the file without applying runtime environment
// overrides. It is used so those overrides are never written back on save.
func loadFileConfig(path string) (*ProxyConfig, error) {
	cfg := &ProxyConfig{}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			applyDefaults(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("stat config: %w", err)
	}
	if err := loadFromYAML(path, cfg); err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}
	applyDefaults(cfg)
	return cfg, nil
}

// persistedView replaces environment-managed fields with the values last read
// from or written to disk. Callers must hold s.mu.
func (s *Store) persistedView(cfg ProxyConfig) ProxyConfig {
	if os.Getenv("NYXPROXY_PREFIX") != "" {
		cfg.Network.IPv6Subnet = s.fileCfg.Network.IPv6Subnet
	}
	if os.Getenv("NYXPROXY_INTERFACE") != "" {
		cfg.Network.InterfaceName = s.fileCfg.Network.InterfaceName
	}
	return cfg
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Snapshot() (ProxyConfig, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg, s.generation
}

func (s *Store) Generation() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation
}

func (s *Store) Prepare(cfg ProxyConfig) (string, error) {
	s.mu.RLock()
	persisted := s.persistedView(cfg)
	s.mu.RUnlock()

	data, err := yaml.Marshal(persisted)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(s.path)
	base := filepath.Base(s.path)
	f, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp config: %w", err)
	}
	tmp := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}
	if err := f.Chmod(0600); err != nil {
		cleanup()
		return "", fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := f.WriteString("# Managed by NyxProxy web console. Manual edits are allowed.\n"); err != nil {
		cleanup()
		return "", fmt.Errorf("write config header: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return "", fmt.Errorf("write temp config: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("sync temp config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("close temp config: %w", err)
	}
	return tmp, nil
}

func (s *Store) Commit(tmpPath string, cfg ProxyConfig) error {
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if runtime.GOOS == "linux" {
		if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
			_ = dir.Sync()
			_ = dir.Close()
		}
	}

	s.mu.Lock()
	s.cfg = cfg
	s.fileCfg = s.persistedView(cfg)
	s.generation++
	s.mu.Unlock()
	return nil
}

func (s *Store) Discard(tmpPath string) {
	if tmpPath != "" {
		_ = os.Remove(tmpPath)
	}
}

func cleanupStaleTemps(path string) {
	pattern := filepath.Join(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	matches, _ := filepath.Glob(pattern)
	for _, match := range matches {
		_ = os.Remove(match)
	}
}
```

- [ ] **Step 6: Set the Go language version**

Configuration files do not need a failing test; verify the effective directive
after the change instead.

```bash
go mod edit -go=1.27.0
go mod tidy
go list -m -f '{{.GoVersion}}'
```

Expected: `1.27.0`, and `go mod tidy` adds no dependencies.

- [ ] **Step 7: Run the configuration tests**

```bash
go test ./internal/config/... -count=1
```

Expected: PASS. If `TestStorePrepareCommitUpdatesGeneration` fails on Windows because of directory sync, confirm `runtime.GOOS == "linux"` guards the directory sync; file sync still runs.

- [ ] **Step 8: Run race and vet checks for the package**

```bash
go test -race ./internal/config/... -count=1
go vet ./internal/config/...
```

Expected: PASS.

- [ ] **Step 9: Commit checkpoint**

```bash
git add go.mod go.sum internal/config
git commit -m "feat(config): add web settings, merge rules, and atomic store"
```

Only run if the user has authorized commits.

## Task 2: Address Command Runner, Pool Interface, and Lifecycle Types

**Files:**
- Create: `pkg/network/runner.go`
- Modify: `pkg/network/ipv6_manager.go`
- Create: `pkg/network/runner_test.go`
- Modify: `pkg/network/interfaces.go`
- Modify: `pkg/network/ndppd_check.go`
- Modify: `pkg/proxy/proxy.go`
- Delete: `pkg/proxy/config.go`
- Modify: `pkg/metrics/metrics.go`

**Interfaces:**
- Produces:
  - `type CommandRunner interface { Run(name string, args ...string) ([]byte, error) }`
  - `func NewIPv6ManagerWithRunner(interfaceName, subnet string, poolSize, maxUsage, maxAgeMinutes int, runner CommandRunner) (*IPv6Manager, error)`
  - `type Pool interface { GetRandomIPv6() (net.IP, error); IsPoolAddress(net.IP) bool; Stop(); GetPoolSize() int; GetTotalRotated() int64; Prefix() string }`
  - `func (m *IPv6Manager) Prefix() string`
  - `func ListInterfaceDetails() ([]InterfaceDetails, error)`
  - `type InterfaceDetails struct { Name string; Addresses []string; SuggestedPrefix string }`
  - `package proxy`: `type State string`, `type ProxyStatus struct`, `type Proxy interface`
  - `func (m *Metrics) SetInterface(name string)`

- [ ] **Step 1: Write failing tests for the command runner and interface details**

Create `pkg/network/runner_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./pkg/network/... -run 'Runner|Populate|Stop|Prefix|InterfaceDetails' -count=1
```

Expected: compile errors for `CommandRunner`, `runner`, `poolSize`, `Prefix`, and `ListInterfaceDetails`.

- [ ] **Step 3: Add the runner abstraction**

Create `pkg/network/runner.go`:

```go
package network

import "os/exec"

// CommandRunner executes system commands. Tests inject a fake to avoid
// touching the host network configuration.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
```

- [ ] **Step 4: Add the pool interface and runner-aware manager**

Replace `pkg/proxy/proxy.go` with the shared lifecycle types (the old HTTP proxy
implementation in this package is unused and is removed). Delete
`pkg/proxy/config.go`:

```go
package proxy

import (
	"context"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
)

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateError    State = "error"
)

type ProxyStatus struct {
	State     State     `json:"state"`
	Type      string    `json:"type"`
	Listen    string    `json:"listen"`
	Protocol  int       `json:"protocol"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error"`
}

type Proxy interface {
	Start() error
	Wait() error
	Stop(ctx context.Context) error
	Status() ProxyStatus
	IPv6Manager() network.Pool
}
```

Add the `Pool` interface to `pkg/network/ipv6_manager.go` near the top:

```go
// Pool is the subset of IPv6Manager used by proxy implementations and status
// reporting. It exists so tests can supply a fake without touching the host.
type Pool interface {
	GetRandomIPv6() (net.IP, error)
	IsPoolAddress(net.IP) bool
	Stop()
	GetPoolSize() int
	GetTotalRotated() int64
	Prefix() string
}
```

Update the `IPv6Manager` struct with `runner CommandRunner` and `poolSize int`:

```go
type IPv6Manager struct {
	mu            sync.Mutex
	interfaceName string
	subnet        *net.IPNet
	ipPool        []net.IP
	ipStats       map[string]*IPStats
	poolIndex     int
	maxUsageCount int
	maxAge        time.Duration
	rotationStop  chan bool
	stopOnce      sync.Once
	totalRotated  int64
	runner        CommandRunner
	poolSize      int
}
```

Replace the constructor entry point so the original signature keeps working:

```go
func NewIPv6Manager(interfaceName string, subnet string, poolSize, maxUsage, maxAgeMinutes int) (*IPv6Manager, error) {
	return NewIPv6ManagerWithRunner(interfaceName, subnet, poolSize, maxUsage, maxAgeMinutes, execRunner{})
}

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
		details, err := ListInterfaceDetails()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-detect IPv6 subnet: %v; set network.ipv6_subnet", err)
		}
		subnet = detectSubnetFromDetails(interfaceName, details)
		if subnet == "" {
			return nil, fmt.Errorf("IPv6 subnet is empty and no /64 address was found on %s; set network.ipv6_subnet", interfaceName)
		}
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
```

Add `Prefix()`:

```go
func (m *IPv6Manager) Prefix() string {
	if m.subnet == nil {
		return ""
	}
	return m.subnet.String()
}
```

Replace the `exec.Command` calls in `assignIPToInterface` and `removeIPFromInterface`:

```go
func (m *IPv6Manager) assignIPToInterface(ip net.IP) error {
	output, err := m.runner.Run("ip", "-6", "addr", "add", fmt.Sprintf("%s/128", ip.String()), "dev", m.interfaceName)
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "File exists") || strings.Contains(outputStr, "RTNETLINK answers: File exists") {
			return nil
		}
		if len(outputStr) > 0 {
			return fmt.Errorf("failed to add IP %s: %v, output: %s", ip.String(), err, outputStr)
		}
		return fmt.Errorf("failed to add IP %s: %v", ip.String(), err)
	}
	return nil
}

func (m *IPv6Manager) removeIPFromInterface(ip string) error {
	output, err := m.runner.Run("ip", "-6", "addr", "del", fmt.Sprintf("%s/128", ip), "dev", m.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to remove IP %s: %v, output: %s", ip, err, string(output))
	}
	return nil
}
```

Remove the `os/exec` import from `ipv6_manager.go`. Replace its `fmt.Printf` calls with `log.Printf`; import `log` instead.

In `Stop`, log pool-cleanup failures without aborting the remaining removals:

```go
for _, ip := range m.ipPool {
	if err := m.removeIPFromInterface(ip.String()); err != nil {
		log.Printf("warning: failed to remove IP %s during cleanup: %v", ip, err)
	}
}
```

In `pkg/network/ndppd_check.go`, replace `fmt.Printf` in
`WarnIfNdppdNotRunning` with `log.Printf` and add the `log` import. The `fmt`
import stays because `CheckNdppdService` returns `fmt.Errorf` values.

- [ ] **Step 5: Add interface details for the API**

Append to `pkg/network/interfaces.go`:

```go
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
```

Add `sort` to the imports.

- [ ] **Step 6: Add the metrics interface setter**

In `pkg/metrics/metrics.go`:

```go
func (m *Metrics) SetInterface(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interfaceName = name
}
```

- [ ] **Step 7: Run the tests**

```bash
go test ./pkg/network/... ./pkg/proxy/... ./pkg/metrics/... -count=1
go vet ./pkg/network/... ./pkg/proxy/... ./pkg/metrics/...
```

Expected: PASS.

- [ ] **Step 8: Commit checkpoint**

```bash
git add pkg/network pkg/proxy pkg/metrics
git commit -m "refactor(network): inject command runner and define proxy lifecycle types"
```

Only run if the user has authorized commits.

---

## Task 3: SOCKS5 Lifecycle Refactor with Listener Ownership and Drain

**Files:**
- Create: `pkg/socks5/listener.go`
- Create: `pkg/socks5/lifecycle_test.go`
- Modify: `pkg/socks5/proxy.go`

**Interfaces:**
- Consumes: `proxy.Proxy`, `proxy.ProxyStatus`, `network.Pool`, `config.ProxyConfig`
- Produces: `func NewProxy(cfg *config.ProxyConfig) (*Proxy, error)` where `*Proxy` implements `proxy.Proxy`.
- Produces: `func (p *Proxy) IPv6Manager() network.Pool` (replaces `GetIPv6Manager`).
- Produces: `func (p *Proxy) Addr() string` for tests and logs.

- [ ] **Step 1: Write failing lifecycle tests**

Create `pkg/socks5/lifecycle_test.go`:

```go
package socks5

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	gosocks5 "github.com/armon/go-socks5"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

func newLifecycleProxy(t *testing.T) *Proxy {
	t.Helper()
	server, err := gosocks5.New(&gosocks5.Config{})
	if err != nil {
		t.Fatalf("new socks5 server: %v", err)
	}
	return &Proxy{
		config: &config.ProxyConfig{Proxy: config.ProxySettings{
			Type:          "socks5",
			ListenAddress: "127.0.0.1",
			ListenPort:    0,
		}},
		server: server,
		state:  proxy.StateStarting,
		done:   make(chan struct{}),
	}
}

func TestStartReturnsAfterBindAndStopClosesListener(t *testing.T) {
	p := newLifecycleProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := p.Addr()
	if addr == "" {
		t.Fatal("Addr is empty")
	}
	if p.Status().State != proxy.StateRunning {
		t.Fatalf("state = %q, want running", p.Status().State)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port was not released: %v", err)
	}
	ln.Close()
}

func TestWaitReturnsNilAfterStop(t *testing.T) {
	p := newLifecycleProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return")
	}
}

func TestStopDrainsActiveConnection(t *testing.T) {
	p := newLifecycleProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The accepted connection is drained, but the client side must observe a
	// closed socket after Stop returns.
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("connection still open after Stop")
	} else if err != io.EOF {
		// A reset is acceptable on some platforms.
		if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			t.Logf("read error after stop: %v", err)
		}
	}
}

func TestStartBindFailureReportsErrorState(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	p := newLifecycleProxy(t)
	p.config.Proxy.ListenPort = port
	if err := p.Start(); err == nil {
		t.Fatal("expected bind failure")
	}
	if p.Status().State != proxy.StateError {
		t.Fatalf("state = %q, want error", p.Status().State)
	}
	if p.Status().Error == "" {
		t.Fatal("expected an error message in status")
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./pkg/socks5/... -run 'Start|Wait|Stop|Bind' -count=1
```

Expected: compile errors for `Addr`, `Status`, `StateRunning`, and `Stop(context.Context)`.

- [ ] **Step 3: Implement the tracking listener**

Create `pkg/socks5/listener.go`:

```go
package socks5

import (
	"context"
	"net"
	"sync"
)

type trackingListener struct {
	net.Listener
	mu      sync.Mutex
	conns   map[*trackedConn]struct{}
	wg      sync.WaitGroup
	closing bool
}

func newTrackingListener(inner net.Listener) *trackingListener {
	return &trackingListener{
		Listener: inner,
		conns:    make(map[*trackedConn]struct{}),
	}
}

func (l *trackingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	tracked := &trackedConn{Conn: conn, listener: l}
	l.conns[tracked] = struct{}{}
	l.wg.Add(1)
	l.mu.Unlock()
	return tracked, nil
}

func (l *trackingListener) Close() error {
	l.mu.Lock()
	l.closing = true
	l.mu.Unlock()
	return l.Listener.Close()
}

// wait blocks until every accepted connection closes or ctx expires. On
// timeout the remaining connections are force-closed so Stop cannot hang.
func (l *trackingListener) wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	l.closeAll()
	<-done
}

func (l *trackingListener) closeAll() {
	l.mu.Lock()
	conns := make([]*trackedConn, 0, len(l.conns))
	for conn := range l.conns {
		conns = append(conns, conn)
	}
	l.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

type trackedConn struct {
	net.Conn
	listener *trackingListener
	once     sync.Once
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.listener.mu.Lock()
		delete(c.listener.conns, c)
		c.listener.mu.Unlock()
		c.listener.wg.Done()
	})
	return c.Conn.Close()
}
```

- [ ] **Step 4: Refactor the SOCKS5 proxy**

In `pkg/socks5/proxy.go`:

1. Remove `var ipv6Manager *network.IPv6Manager`.
2. Change `Proxy` to:

```go
type Proxy struct {
	config   *config.ProxyConfig
	server   *gosocks5.Server
	pool     network.Pool
	resolver *net.Resolver

	mu        sync.Mutex
	listener  *trackingListener
	state     proxy.State
	lastError string
	startedAt time.Time
	serveErr  error
	done      chan struct{}
}
```

Import the library with an alias so the package name and interface package do not collide:

```go
gosocks5 "github.com/armon/go-socks5"
"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
```

Replace every `socks5.` library reference with `gosocks5.`.

3. In `NewProxy`, replace `mgr` with `network.Pool`:

```go
var pool network.Pool
if cfg.Network.RotateIPv6 && cfg.Network.IPv6Subnet != "" {
	mgr, err := network.NewIPv6Manager(
		cfg.Network.InterfaceName,
		cfg.Network.IPv6Subnet,
		cfg.Network.IPv6PoolSize,
		cfg.Network.IPv6MaxUsage,
		cfg.Network.IPv6MaxAge,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 manager: %v", err)
	}
	pool = mgr
	log.Printf("IPv6 rotation enabled for subnet: %s", cfg.Network.IPv6Subnet)
}
```

Use `pool` everywhere `mgr` was used. In the `isPooled` closure, change to:

```go
var isPooled func(net.IP) bool
if pool != nil {
	isPooled = pool.IsPoolAddress
}
```

And `if pool == nil { ... }`.

Construct:

```go
proxy := &Proxy{
	config:   cfg,
	pool:     pool,
	resolver: createResolver(dnsSource, protocol),
	state:    proxy.StateStarting,
	done:     make(chan struct{}),
}
```

On constructor error after the pool exists, call `pool.Stop()`. On `gosocks5.New` error, do the same.

4. Replace `p.ipv6Manager` with `p.pool` in `getNextLocalAddr`.

5. Replace `Start`, `Stop`, and `GetIPv6Manager` with:

```go
func (p *Proxy) Start() error {
	addr := net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	p.debugLog(1, "Starting socks5 proxy on %s (Protocol: IPv%d)", addr, p.config.GetProxyProtocol())

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.setError(err)
		if p.pool != nil {
			p.pool.Stop()
		}
		return fmt.Errorf("failed to create listener: %v", err)
	}

	p.mu.Lock()
	p.listener = newTrackingListener(ln)
	p.state = proxy.StateRunning
	p.startedAt = time.Now()
	p.lastError = ""
	p.mu.Unlock()

	go func() {
		err := p.server.Serve(p.listener)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		p.mu.Lock()
		p.serveErr = err
		if err != nil {
			p.state = proxy.StateError
			p.lastError = err.Error()
		}
		close(p.done)
		p.mu.Unlock()
	}()
	return nil
}

func (p *Proxy) Wait() error {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.serveErr
}

func (p *Proxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	ln := p.listener
	if ln == nil {
		p.mu.Unlock()
		return nil
	}
	p.listener = nil
	p.mu.Unlock()

	_ = ln.Close()
	ln.wait(ctx)

	if p.pool != nil {
		p.pool.Stop()
	}

	p.mu.Lock()
	if p.state != proxy.StateError {
		p.state = proxy.StateStopped
	}
	p.mu.Unlock()
	return nil
}

func (p *Proxy) Status() proxy.ProxyStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	started := p.startedAt
	state := p.state
	if state == "" {
		state = proxy.StateStarting
	}
	listen := ""
	if p.listener != nil {
		listen = p.listener.Addr().String()
	} else if p.config != nil {
		listen = net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	}
	protocol := 4
	if p.config != nil {
		protocol = p.config.GetProxyProtocol()
	}
	kind := "socks5"
	if p.config != nil && p.config.GetProxyType() != "" {
		kind = p.config.GetProxyType()
	}
	return proxy.ProxyStatus{
		State:     state,
		Type:      kind,
		Listen:    listen,
		Protocol:  protocol,
		StartedAt: started,
		Error:     p.lastError,
	}
}

func (p *Proxy) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

func (p *Proxy) IPv6Manager() network.Pool {
	return p.pool
}

func (p *Proxy) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = proxy.StateError
	p.lastError = err.Error()
}
```

Add imports: `errors`, `strconv`, `sync`, and the `proxy` package. Remove the `context` import if it is only used elsewhere (it is used by customResolver and dialer, so keep it).

`Start` uses `p.listener` after unlocking; another goroutine could call Stop and set it nil before `Serve` reads it. Capture the listener in a local:

```go
tracked := newTrackingListener(ln)
p.mu.Lock()
p.listener = tracked
...
p.mu.Unlock()
go func() {
	err := p.server.Serve(tracked)
	...
}()
```

Update the implementation accordingly.

Also `Serve` returns `net.ErrClosed` wrapped? It returns `net.ErrClosed` directly. Use `errors.Is`.

- [ ] **Step 5: Run the tests**

```bash
go test ./pkg/socks5/... -count=1
go test -race ./pkg/socks5/... -count=1
go vet ./pkg/socks5/...
```

Expected: PASS. If `TestStopDrainsActiveConnection` hangs, verify `trackedConn.Close` is called by the SOCKS5 server when the client disconnects; close the client connection in the test before calling `Stop` if needed.

- [ ] **Step 6: Commit checkpoint**

```bash
git add pkg/socks5
git commit -m "refactor(socks5): own the listener and implement drainable lifecycle"
```

Only run if the user has authorized commits.

---

## Task 4: HTTPS and Monitoring Lifecycle Refactor

**Files:**
- Create: `pkg/https/lifecycle_test.go`
- Modify: `pkg/https/proxy.go`
- Create: `pkg/monitoring/lifecycle_test.go`
- Modify: `pkg/monitoring/server.go`

**Interfaces:**
- Consumes: `proxy.Proxy`, `proxy.ProxyStatus`, `network.Pool`
- Produces: `*https.Proxy` implements `proxy.Proxy`; `func (p *Proxy) IPv6Manager() network.Pool`; `func (p *Proxy) Addr() string`.
- Produces: `func (s *Server) Addr() string` for monitoring tests.
- Produces: monitoring `Start()` is non-blocking and `Stop()` is idempotent.

- [ ] **Step 1: Write failing HTTPS lifecycle tests**

Create `pkg/https/lifecycle_test.go`:

```go
package https

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

func newHTTPSProxy(t *testing.T) *Proxy {
	t.Helper()
	cfg := &config.ProxyConfig{Proxy: config.ProxySettings{
		Type:          "https",
		ListenAddress: "127.0.0.1",
		ListenPort:    0,
	}}
	return &Proxy{
		config: cfg,
		httpServer: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		},
		state: proxy.StateStarting,
		done:  make(chan struct{}),
	}
}

func TestHTTPSStartReturnsAfterBind(t *testing.T) {
	p := newHTTPSProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := p.Addr()
	if addr == "" {
		t.Fatal("Addr is empty")
	}
	if p.Status().State != proxy.StateRunning {
		t.Fatalf("state = %q", p.Status().State)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port was not released: %v", err)
	}
	ln.Close()
}

func TestHTTPSWaitReturnsNilAfterStop(t *testing.T) {
	p := newHTTPSProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return")
	}
}
```

- [ ] **Step 2: Write failing monitoring lifecycle tests**

Create `pkg/monitoring/lifecycle_test.go`:

```go
package monitoring

import (
	"testing"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
)

func TestMonitoringStartReturnsAndStopIsIdempotent(t *testing.T) {
	cfg := &config.ProxyConfig{}
	cfg.Monitoring.Enabled = true
	cfg.Monitoring.Port = 0

	srv := New(cfg, metrics.New("test"), "test")
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.Addr() == "" {
		t.Fatal("Addr is empty")
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
```

- [ ] **Step 3: Run the tests and verify they fail**

```bash
go test ./pkg/https/... ./pkg/monitoring/... -run 'Lifecycle|Start|Wait' -count=1
```

Expected: compile errors for `state`, `done`, `Status`, `Addr`, and `Stop(context.Context)`.

- [ ] **Step 4: Refactor the HTTPS proxy**

In `pkg/https/proxy.go`:

1. Remove `var ipv6Manager *network.IPv6Manager`.
2. Add imports: `errors`, `log`, `sync`, and the `proxy` package.
3. Change `debugLog` to use `log.Printf`.
4. Change the struct:

```go
type Proxy struct {
	config     *config.ProxyConfig
	httpServer *http.Server
	resolver   *net.Resolver
	localAddr  net.IP
	pool       network.Pool

	mu        sync.Mutex
	listener  net.Listener
	state     proxy.State
	lastError string
	startedAt time.Time
	serveErr  error
	done      chan struct{}
}
```

5. In `NewProxy`, replace `mgr` with `pool network.Pool` exactly as in Task 3. Pass `pool` into the struct. On `GetOutboundIP` failure, call `pool.Stop()`.
6. Use `p.pool` in `getNextLocalAddr`.
7. Replace `Start`, `Stop`, `GetIPv6Manager` with:

```go
func (p *Proxy) Start() error {
	addr := net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	p.debugLog(1, "Starting https proxy on %s (Protocol: IPv%d)", addr, p.config.GetProxyProtocol())

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.setError(err)
		if p.pool != nil {
			p.pool.Stop()
		}
		return fmt.Errorf("failed to create listener: %v", err)
	}

	p.mu.Lock()
	p.listener = ln
	p.httpServer.Addr = addr
	p.state = proxy.StateRunning
	p.startedAt = time.Now()
	p.lastError = ""
	done := p.done
	p.mu.Unlock()

	go func() {
		err := p.httpServer.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		p.mu.Lock()
		p.serveErr = err
		if err != nil {
			p.state = proxy.StateError
			p.lastError = err.Error()
		}
		close(done)
		p.mu.Unlock()
	}()
	return nil
}

func (p *Proxy) Wait() error {
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	<-done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.serveErr
}

func (p *Proxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	if p.listener == nil {
		p.mu.Unlock()
		return nil
	}
	p.listener = nil
	p.mu.Unlock()

	err := p.httpServer.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	_ = p.httpServer.Close()
	if p.pool != nil {
		p.pool.Stop()
	}

	p.mu.Lock()
	if p.state != proxy.StateError {
		p.state = proxy.StateStopped
	}
	p.mu.Unlock()
	return err
}

func (p *Proxy) Status() proxy.ProxyStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	listen := ""
	if p.listener != nil {
		listen = p.listener.Addr().String()
	} else if p.config != nil {
		listen = net.JoinHostPort(p.config.GetListenAddress(), strconv.Itoa(p.config.GetListenPort()))
	}
	state := p.state
	if state == "" {
		state = proxy.StateStarting
	}
	kind := "https"
	if p.config != nil && p.config.GetProxyType() != "" {
		kind = p.config.GetProxyType()
	}
	protocol := 4
	if p.config != nil {
		protocol = p.config.GetProxyProtocol()
	}
	return proxy.ProxyStatus{
		State:     state,
		Type:      kind,
		Listen:    listen,
		Protocol:  protocol,
		StartedAt: p.startedAt,
		Error:     p.lastError,
	}
}

func (p *Proxy) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

func (p *Proxy) IPv6Manager() network.Pool {
	return p.pool
}

func (p *Proxy) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = proxy.StateError
	p.lastError = err.Error()
}
```

The test builds `Proxy` with `state` and `done`; `Start` uses `p.done`. Ensure `NewProxy` sets `done: make(chan struct{})` as well.

- [ ] **Step 5: Refactor monitoring to non-blocking Start**

In `pkg/monitoring/server.go`:

Add fields:

```go
type Server struct {
	config  *config.ProxyConfig
	metrics *metrics.Metrics
	server  *http.Server
	version string
	mu      sync.Mutex
	listener net.Listener
}
```

Replace `Start` with:

```go
func (s *Server) Start() error {
	if !s.config.Monitoring.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/config", s.handleConfig)

	listenAddr := "127.0.0.1"
	if s.config.Monitoring.AllowRemote {
		listenAddr = "0.0.0.0"
	}
	addr := net.JoinHostPort(listenAddr, strconv.Itoa(s.config.Monitoring.Port))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("monitoring listen: %w", err)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.mu.Lock()
	s.listener = ln
	s.server = srv
	s.mu.Unlock()

	log.Printf("Starting monitoring server on %s", ln.Addr().String())
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("Monitoring server error: %v", err)
		}
	}()
	return nil
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.server
	s.server = nil
	s.listener = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	err := srv.Close()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
```

Add imports: `errors`, `strconv`, `sync`. Remove the `fmt.Sprintf` port formatting.

- [ ] **Step 6: Run the tests**

```bash
go test ./pkg/https/... ./pkg/monitoring/... -count=1
go test -race ./pkg/https/... ./pkg/monitoring/... -count=1
go vet ./pkg/https/... ./pkg/monitoring/...
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

```bash
git add pkg/https pkg/monitoring
git commit -m "refactor(proxy): add lifecycle and graceful shutdown to https and monitoring"
```

Only run if the user has authorized commits.

## Task 5: Log Broker with Ring Buffer and SSE Subscriptions

**Files:**
- Create: `internal/logbuf/broker.go`
- Create: `internal/logbuf/broker_test.go`

**Interfaces:**
- Produces:
  - `type Record struct { Seq uint64; TS time.Time; Line string }`
  - `func New(size, maxSubs int) *Broker`
  - `func (b *Broker) Write(p []byte) (int, error)`
  - `func (b *Broker) Subscribe() (<-chan Record, func(), bool)`
  - `func (b *Broker) Replay(after uint64) ([]Record, uint64)`
  - `func (b *Broker) Close()`

- [ ] **Step 1: Write failing broker tests**

Create `internal/logbuf/broker_test.go`:

```go
package logbuf

import (
	"fmt"
	"strings"
	"testing"
)

func TestWriteSplitsLinesAndAssignsSequence(t *testing.T) {
	b := New(10, 1)
	if _, err := b.Write([]byte("first\nsecond\nthi")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("rd\n")); err != nil {
		t.Fatal(err)
	}

	got, dropped := b.Replay(0)
	if dropped != 0 {
		t.Fatalf("dropped = %d", dropped)
	}
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3: %#v", len(got), got)
	}
	lines := []string{got[0].Line, got[1].Line, got[2].Line}
	if strings.Join(lines, ",") != "first,second,third" {
		t.Fatalf("lines = %v", lines)
	}
	if got[0].Seq != 1 || got[2].Seq != 3 {
		t.Fatalf("sequence = %d..%d", got[0].Seq, got[2].Seq)
	}
}

func TestRingBufferDropsOldestRecords(t *testing.T) {
	b := New(3, 1)
	for i := 0; i < 5; i++ {
		b.Write([]byte(fmt.Sprintf("line-%d\n", i)))
	}
	got, _ := b.Replay(0)
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	if got[0].Line != "line-2" || got[2].Line != "line-4" {
		t.Fatalf("unexpected records: %#v", got)
	}
}

func TestReplayReportsGapForExpiredLastEventID(t *testing.T) {
	b := New(3, 1)
	for i := 0; i < 10; i++ {
		b.Write([]byte(fmt.Sprintf("line-%d\n", i)))
	}
	got, dropped := b.Replay(2)
	if dropped != 5 {
		t.Fatalf("dropped = %d, want 5", dropped)
	}
	if len(got) != 3 || got[0].Seq != 8 {
		t.Fatalf("records = %#v", got)
	}
}

func TestReplayAfterLatestReturnsNothing(t *testing.T) {
	b := New(3, 1)
	b.Write([]byte("one\n"))
	got, dropped := b.Replay(1)
	if len(got) != 0 || dropped != 0 {
		t.Fatalf("got=%#v dropped=%d", got, dropped)
	}
}

func TestSlowSubscriberIsRemovedWithoutBlocking(t *testing.T) {
	b := New(10, 1)
	ch, cancel, ok := b.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()

	for i := 0; i < 300; i++ {
		if _, err := b.Write([]byte(fmt.Sprintf("line-%d\n", i))); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	b.mu.Lock()
	subs := len(b.subs)
	b.mu.Unlock()
	if subs != 0 {
		t.Fatalf("slow subscriber still registered: %d", subs)
	}
	select {
	case _, open := <-ch:
		if open {
			// Buffered records may still be readable; drain them.
			for range ch {
			}
		}
	default:
	}
}

func TestSubscribeRejectsBeyondLimit(t *testing.T) {
	b := New(10, 1)
	_, cancel, ok := b.Subscribe()
	if !ok {
		t.Fatal("first subscribe failed")
	}
	defer cancel()
	if _, _, ok := b.Subscribe(); ok {
		t.Fatal("second subscribe must be rejected")
	}
}

func TestLastSeqTracksNewestRecord(t *testing.T) {
	b := New(2, 1)
	if got := b.LastSeq(); got != 0 {
		t.Fatalf("empty LastSeq = %d, want 0", got)
	}
	_, _ = b.Write([]byte("one\ntwo\nthree\n"))
	if got := b.LastSeq(); got != 3 {
		t.Fatalf("LastSeq = %d, want 3", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./internal/logbuf/... -count=1
```

Expected: compile errors for `New`, `Broker`, and methods.

- [ ] **Step 3: Implement the broker**

Create `internal/logbuf/broker.go`:

```go
package logbuf

import (
	"bytes"
	"sync"
	"time"
)

type Record struct {
	Seq  uint64    `json:"seq"`
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

type Broker struct {
	mu      sync.Mutex
	ring    []Record
	max     int
	nextSeq uint64
	partial []byte
	subs    map[chan Record]struct{}
	maxSubs int
	closed  bool
}

func New(size, maxSubs int) *Broker {
	if size <= 0 {
		size = 2000
	}
	if maxSubs <= 0 {
		maxSubs = 8
	}
	return &Broker{
		ring:    make([]Record, 0, size),
		max:     size,
		subs:    make(map[chan Record]struct{}),
		maxSubs: maxSubs,
	}
}

func (b *Broker) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return len(p), nil
	}

	b.partial = append(b.partial, p...)
	for {
		idx := bytes.IndexByte(b.partial, '\n')
		if idx < 0 {
			break
		}
		line := string(b.partial[:idx])
		b.partial = b.partial[idx+1:]
		b.appendLocked(line)
	}
	if len(b.partial) > 64*1024 {
		b.appendLocked(string(b.partial))
		b.partial = nil
	}
	return len(p), nil
}

func (b *Broker) appendLocked(line string) {
	b.nextSeq++
	record := Record{Seq: b.nextSeq, TS: time.Now(), Line: line}
	if len(b.ring) < b.max {
		b.ring = append(b.ring, record)
	} else {
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = record
	}
	for ch := range b.subs {
		select {
		case ch <- record:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *Broker) Subscribe() (<-chan Record, func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.subs) >= b.maxSubs {
		return nil, nil, false
	}
	ch := make(chan Record, 256)
	b.subs[ch] = struct{}{}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return ch, cancel, true
}

// Replay returns records newer than after. When after falls outside the ring,
// dropped reports how many sequence numbers were lost.
func (b *Broker) Replay(after uint64) ([]Record, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) == 0 {
		return nil, 0
	}
	last := b.ring[len(b.ring)-1].Seq
	if after >= last {
		return nil, 0
	}
	start := 0
	for start < len(b.ring) && b.ring[start].Seq <= after {
		start++
	}
	var dropped uint64
	if after != 0 && b.ring[0].Seq > after+1 {
		dropped = b.ring[0].Seq - after - 1
	}
	out := make([]Record, len(b.ring)-start)
	copy(out, b.ring[start:])
	return out, dropped
}

// LastSeq returns the newest sequence number, or 0 when nothing was logged.
func (b *Broker) LastSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) == 0 {
		return 0
	}
	return b.ring[len(b.ring)-1].Seq
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/logbuf/... -count=1
go test -race ./internal/logbuf/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit checkpoint**

```bash
git add internal/logbuf
git commit -m "feat(logbuf): add ring buffer and non-blocking log subscriptions"
```

Only run if the user has authorized commits.

---

## Task 6: Supervisor with Transactional Apply and Rollback

**Files:**
- Create: `internal/app/supervisor.go`
- Create: `internal/app/status.go`
- Create: `internal/app/errors.go`
- Create: `internal/app/supervisor_test.go`

**Interfaces:**
- Consumes:
  - `config.Store`, `config.ProxyConfig`, `config.MergeUpdate`, `config.ValidationError`
  - `proxy.Proxy`, `monitoring.Server`, `metrics.Metrics`
  - `socks5.NewProxy`, `https.NewProxy`
- Produces:
  - `type Factory func(cfg *config.ProxyConfig) (proxy.Proxy, error)`
  - `func DefaultFactory(cfg *config.ProxyConfig) (proxy.Proxy, error)`
  - `func New(cfg config.ProxyConfig, store *config.Store, factory Factory, m *metrics.Metrics, buildVersion string) *Supervisor`
  - `func (s *Supervisor) Start() error`
  - `func (s *Supervisor) Apply(ctx context.Context, submitted config.ProxyConfig) (ApplyResult, error)`
  - `func (s *Supervisor) Retry(ctx context.Context) error`
  - `func (s *Supervisor) Shutdown(ctx context.Context)`
  - `func (s *Supervisor) Status() Status`
  - `func (s *Supervisor) Config() (config.ProxyConfig, uint64)`
  - `var ErrApplyInProgress`, `var ErrProxyAlreadyRunning`
  - `type ApplyError struct { Code, Message string; Fields map[string]string; RolledBack bool }`

- [ ] **Step 1: Write failing supervisor tests**

Create `internal/app/supervisor_test.go`:

```go
package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
)

type fakeProxy struct {
	cfg      config.ProxyConfig
	startErr error
	done     chan struct{}
	mu       sync.Mutex
	state    proxy.State
	stopped  bool
}

func newFakeProxy(cfg *config.ProxyConfig) *fakeProxy {
	return &fakeProxy{cfg: *cfg, done: make(chan struct{}), state: proxy.StateStarting}
}

func (f *fakeProxy) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		f.state = proxy.StateError
		return f.startErr
	}
	f.state = proxy.StateRunning
	return nil
}

func (f *fakeProxy) Wait() error {
	<-f.done
	return nil
}

func (f *fakeProxy) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped {
		f.stopped = true
		close(f.done)
	}
	f.state = proxy.StateStopped
	return nil
}

func (f *fakeProxy) Status() proxy.ProxyStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return proxy.ProxyStatus{State: f.state, Type: f.cfg.GetProxyType()}
}

func (f *fakeProxy) IPv6Manager() network.Pool { return nil }

type scriptedFactory struct {
	mu      sync.Mutex
	calls   int
	failOn  int
	created []*fakeProxy
}

func (f *scriptedFactory) create(cfg *config.ProxyConfig) (proxy.Proxy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	p := newFakeProxy(cfg)
	if f.calls == f.failOn {
		p.startErr = errors.New("boom")
	}
	f.created = append(f.created, p)
	return p, nil
}

func baseConfig() config.ProxyConfig {
	return config.ProxyConfig{
		Proxy: config.ProxySettings{
			Type:          "socks5",
			ListenAddress: "127.0.0.1",
			ListenPort:    1080,
			Username:      "admin",
			Password:      "secret",
		},
		Network: config.NetworkSettings{
			IPv4Enabled:  true,
			RotateIPv6:   false,
			IPv6PoolSize: 4,
			IPv6MaxUsage: 10,
			IPv6MaxAge:   30,
		},
		Monitoring: config.MonitoringSettings{Enabled: false},
		Logging:    config.LoggingSettings{DebugLevel: 1},
		Web:        config.WebSettings{Enabled: false, ListenAddress: "127.0.0.1", Port: 18081, Username: "admin"},
	}
}

func newTestStore(t *testing.T, cfg config.ProxyConfig) (*config.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(renderTestConfig(t, cfg)), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, path
}

func renderTestConfig(t *testing.T, cfg config.ProxyConfig) string {
	t.Helper()
	return strings.Join([]string{
		"proxy:",
		"  type: " + cfg.Proxy.Type,
		"  listen_address: " + cfg.Proxy.ListenAddress,
		"  listen_port: 1080",
		"  username: " + cfg.Proxy.Username,
		"  password: " + cfg.Proxy.Password,
		"network:",
		"  ipv4_enabled: true",
		"  ipv6_enabled: false",
		"  rotate_ipv6: false",
		"monitoring:",
		"  enabled: false",
		"logging:",
		"  debug_level: 1",
		"web:",
		"  enabled: false",
		"  listen_address: 127.0.0.1",
		"  port: 18081",
		"  username: admin",
	}, "\n") + "\n"
}

func startSupervisor(t *testing.T, factory *scriptedFactory) (*Supervisor, *config.Store, string) {
	t.Helper()
	cfg := baseConfig()
	store, path := newTestStore(t, cfg)
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sup.Shutdown(ctx)
	})
	return sup, store, path
}

func TestApplyRestartsProxyAndPersists(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, path := startSupervisor(t, factory)

	cfg, _ := store.Snapshot()
	submitted := cfg
	submitted.Proxy.ListenPort = 1081

	result, err := sup.Apply(context.Background(), submitted)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed || !result.Restarted.Proxy {
		t.Fatalf("result = %+v", result)
	}
	if result.Generation != 2 {
		t.Fatalf("generation = %d, want 2", result.Generation)
	}
	if factory.calls != 2 {
		t.Fatalf("factory calls = %d, want 2", factory.calls)
	}
	if !factory.created[0].stopped {
		t.Fatal("old proxy was not stopped")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "listen_port: 1081") {
		t.Fatalf("config not persisted:\n%s", data)
	}
	got, gen := store.Snapshot()
	if got.Proxy.ListenPort != 1081 || gen != 2 {
		t.Fatalf("snapshot = %+v gen=%d", got.Proxy, gen)
	}
}

func TestApplyRollsBackWhenNewProxyFails(t *testing.T) {
	factory := &scriptedFactory{failOn: 2}
	sup, store, path := startSupervisor(t, factory)

	cfg, _ := store.Snapshot()
	submitted := cfg
	submitted.Proxy.ListenPort = 1081

	_, err := sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) {
		t.Fatalf("error = %T %v, want *ApplyError", err, err)
	}
	if applyErr.Code != "apply_failed" || !applyErr.RolledBack {
		t.Fatalf("apply error = %+v", applyErr)
	}
	if factory.calls != 3 {
		t.Fatalf("factory calls = %d, want 3 (start, fail, rollback)", factory.calls)
	}
	if sup.Status().Proxy.State != "running" {
		t.Fatalf("proxy state = %q", sup.Status().Proxy.State)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "listen_port: 1080") {
		t.Fatalf("config changed despite rollback:\n%s", data)
	}
	if _, gen := store.Snapshot(); gen != 1 {
		t.Fatalf("generation = %d, want 1", gen)
	}
}

func TestApplyMonitoringFailureKeepsProxyRunning(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	_, portText, _ := net.SplitHostPort(occupied.Addr().String())
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	submitted, _ := store.Snapshot()
	submitted.Monitoring.Enabled = true
	submitted.Monitoring.Port = port

	_, err = sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "apply_failed" {
		t.Fatalf("error = %v, want apply_failed", err)
	}
	status := sup.Status()
	if status.Proxy.State != "running" {
		t.Fatalf("proxy state = %q, want running", status.Proxy.State)
	}
	if status.Monitoring.State != "stopped" {
		t.Fatalf("monitoring state = %q, want stopped after rollback", status.Monitoring.State)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1 (monitoring-only change)", factory.calls)
	}
}

func TestApplyWithNoChangesDoesNotRestart(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	cfg, _ := store.Snapshot()

	result, err := sup.Apply(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Changed {
		t.Fatal("changed = true, want false")
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
}

func TestApplyRejectsLockedField(t *testing.T) {
	t.Setenv("NYXPROXY_PREFIX", "2001:db8:ffff::/64")
	factory := &scriptedFactory{}
	cfg := baseConfig()
	cfg.Network.IPv6Subnet = "2001:db8:ffff::/64"
	store, _ := newTestStore(t, cfg)
	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatal(err)
	}

	submitted, _ := store.Snapshot()
	submitted.Network.IPv6Subnet = "2001:db8:eeee::/64"
	_, err := sup.Apply(context.Background(), submitted)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "field_locked" {
		t.Fatalf("error = %v", err)
	}
	if applyErr.Fields["network.ipv6_subnet"] == "" {
		t.Fatalf("fields = %#v", applyErr.Fields)
	}
}

func TestConcurrentApplyReturnsInProgress(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)
	cfg, _ := store.Snapshot()

	sup.applyMu.Lock()
	_, err := sup.Apply(context.Background(), cfg)
	sup.applyMu.Unlock()
	if !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("error = %v, want ErrApplyInProgress", err)
	}
}

func TestRetryStartsProxyAfterStartupError(t *testing.T) {
	factory := &scriptedFactory{failOn: 1}
	cfg := baseConfig()
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "127.0.0.1"
	cfg.Web.Port = 18081
	cfg.Web.Username = "admin"
	cfg.Web.Password = "web-secret"
	store, _ := newTestStore(t, cfg)

	sup := New(cfg, store, factory.create, metrics.New("test"), "test")
	if err := sup.Start(); err != nil {
		t.Fatalf("Start with web enabled must not fail: %v", err)
	}
	if sup.Status().Proxy.State != "error" {
		t.Fatalf("state = %q, want error", sup.Status().Proxy.State)
	}
	if err := sup.Retry(context.Background()); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if sup.Status().Proxy.State != "running" {
		t.Fatalf("state = %q, want running", sup.Status().Proxy.State)
	}
}

func TestRetryRejectsRunningProxy(t *testing.T) {
	factory := &scriptedFactory{}
	sup, _, _ := startSupervisor(t, factory)
	if err := sup.Retry(context.Background()); !errors.Is(err, ErrProxyAlreadyRunning) {
		t.Fatalf("error = %v, want ErrProxyAlreadyRunning", err)
	}
}

func TestApplyAfterShutdownFails(t *testing.T) {
	factory := &scriptedFactory{}
	sup, store, _ := startSupervisor(t, factory)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sup.Shutdown(ctx)
	if state := sup.Status().Proxy.State; state != "stopped" {
		t.Fatalf("proxy state = %q, want stopped", state)
	}

	cfg, _ := store.Snapshot()
	cfg.Proxy.ListenPort = 1081
	_, err := sup.Apply(context.Background(), cfg)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code != "apply_failed" {
		t.Fatalf("error = %v, want apply_failed", err)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./internal/app/... -count=1
```

Expected: compile errors for `New`, `Supervisor`, `ApplyError`, and the error variables.

- [ ] **Step 3: Implement the error types**

Create `internal/app/errors.go`:

```go
package app

import "errors"

var ErrApplyInProgress = errors.New("a configuration apply is already in progress")
var ErrProxyAlreadyRunning = errors.New("proxy is already running")

type ApplyError struct {
	Code       string
	Message    string
	Fields     map[string]string
	RolledBack bool
}

func (e *ApplyError) Error() string {
	return e.Message
}
```

- [ ] **Step 4: Implement the status DTOs**

Create `internal/app/status.go`:

```go
package app

import "time"

type Status struct {
	Version       string           `json:"version"`
	UptimeSeconds int64            `json:"uptime_seconds"`
	Proxy         ProxyStatus      `json:"proxy"`
	Monitoring    MonitoringStatus `json:"monitoring"`
	Pool          PoolStatus       `json:"pool"`
	Metrics       MetricsStatus    `json:"metrics"`
	Config        ConfigStatus     `json:"config"`
	Platform      PlatformStatus   `json:"platform"`
	Apply         ApplyStatus      `json:"apply"`
}

type ProxyStatus struct {
	State     string     `json:"state"`
	Type      string     `json:"type"`
	Listen    string     `json:"listen"`
	Protocol  int        `json:"protocol"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Error     string     `json:"error"`
}

type MonitoringStatus struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
	Error   string `json:"error"`
}

type PoolStatus struct {
	Enabled       bool   `json:"enabled"`
	Size          int    `json:"size"`
	Prefix        string `json:"prefix"`
	Rotations     int64  `json:"rotations"`
	MaxUsage      int    `json:"max_usage"`
	MaxAgeMinutes int    `json:"max_age_minutes"`
}

type MetricsStatus struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	TotalRequests     int64 `json:"total_requests"`
	BytesSent         int64 `json:"bytes_sent"`
	BytesReceived     int64 `json:"bytes_received"`
}

type LastApply struct {
	OK         bool      `json:"ok"`
	Error      string    `json:"error"`
	At         time.Time `json:"at"`
	Generation uint64    `json:"generation"`
}

type ConfigStatus struct {
	Path          string     `json:"path"`
	Generation    uint64     `json:"generation"`
	LastAppliedAt *time.Time `json:"last_applied_at,omitempty"`
	LastApply     LastApply  `json:"last_apply"`
}

type PlatformStatus struct {
	OS        string   `json:"os"`
	IPCommand bool     `json:"ip_command"`
	Warnings  []string `json:"warnings"`
}

type ApplyStatus struct {
	InProgress bool   `json:"in_progress"`
	Step       string `json:"step"`
}

type ApplyResult struct {
	Changed        bool      `json:"changed"`
	Generation     uint64    `json:"generation"`
	Restarted      Restarted `json:"restarted"`
	ReauthRequired bool      `json:"reauth_required"`
}

type Restarted struct {
	Proxy      bool `json:"proxy"`
	Monitoring bool `json:"monitoring"`
}
```

- [ ] **Step 5: Implement the supervisor**

Create `internal/app/supervisor.go`:

```go
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/https"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/monitoring"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/proxy"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/socks5"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/version"
)

type Factory func(cfg *config.ProxyConfig) (proxy.Proxy, error)

func DefaultFactory(cfg *config.ProxyConfig) (proxy.Proxy, error) {
	switch cfg.GetProxyType() {
	case "socks5":
		return socks5.NewProxy(cfg)
	case "https":
		return https.NewProxy(cfg)
	default:
		return nil, fmt.Errorf("unsupported proxy type %q", cfg.GetProxyType())
	}
}

type Supervisor struct {
	mu      sync.Mutex
	applyMu sync.Mutex

	store        *config.Store
	cfg          config.ProxyConfig
	factory      Factory
	metrics      *metrics.Metrics
	buildVersion string

	startedAt time.Time
	current   proxy.Proxy
	proxyState string
	proxyErr   string

	monitoring      *monitoring.Server
	monitoringState string
	monitoringErr   string

	applyInProgress bool
	applyStep       string
	lastAppliedAt   time.Time
	lastApply       LastApply

	shuttingDown bool
}

func New(cfg config.ProxyConfig, store *config.Store, factory Factory, m *metrics.Metrics, buildVersion string) *Supervisor {
	if factory == nil {
		factory = DefaultFactory
	}
	if m == nil {
		m = metrics.New(cfg.Network.InterfaceName)
	}
	return &Supervisor{
		store:           store,
		cfg:             cfg,
		factory:         factory,
		metrics:         m,
		buildVersion:    buildVersion,
		proxyState:      string(proxy.StateStopped),
		monitoringState: "stopped",
	}
}

func (s *Supervisor) Start() error {
	s.mu.Lock()
	s.startedAt = time.Now()
	cfg := s.cfg
	s.proxyState = string(proxy.StateStarting)
	s.mu.Unlock()

	if cfg.Monitoring.Enabled {
		if err := s.startMonitoring(&cfg); err != nil {
			log.Printf("monitoring server failed to start: %v", err)
			s.mu.Lock()
			s.monitoringState = "error"
			s.monitoringErr = err.Error()
			s.mu.Unlock()
		}
	}

	p, err := s.factory(&cfg)
	if err == nil && p == nil {
		err = errors.New("proxy factory returned nil")
	}
	if err == nil {
		err = p.Start()
	}
	if err != nil {
		if p != nil {
			stopProxy(p)
		}
		s.setProxyError(err)
		if cfg.Web.Enabled {
			log.Printf("proxy failed to start; web console remains available: %v", err)
			return nil
		}
		return err
	}
	if !s.adoptProxy(p) {
		return context.Canceled
	}
	return nil
}

func (s *Supervisor) Apply(ctx context.Context, submitted config.ProxyConfig) (ApplyResult, error) {
	if !s.applyMu.TryLock() {
		return ApplyResult{}, ErrApplyInProgress
	}
	defer s.applyMu.Unlock()
	if s.isShuttingDown() {
		return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
	}
	s.setApply(true, "validating")
	defer s.setApply(false, "")

	current, _ := s.store.Snapshot()
	merged, err := config.MergeUpdate(current, submitted)
	if err != nil {
		fields := validationFields(err)
		code := "field_locked"
		for _, message := range fields {
			if message == "requires restart" {
				code = "restart_required"
				break
			}
		}
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: code, Message: err.Error(), Fields: fields}
	}
	if err := merged.Validate(); err != nil {
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "validation_failed", Message: "configuration is invalid", Fields: validationFields(err)}
	}

	proxyChanged := current.Proxy != merged.Proxy || current.Network != merged.Network || current.Logging != merged.Logging
	monitoringChanged := current.Monitoring != merged.Monitoring
	reauthRequired := current.Web.Username != merged.Web.Username || current.Web.Password != merged.Web.Password

	if !proxyChanged && !monitoringChanged && !reauthRequired {
		s.mu.Lock()
		p := s.current
		s.mu.Unlock()
		if p == nil || p.Status().State == proxy.StateError {
			s.setApply(true, "starting_proxy")
			if err := s.restartProxy(ctx, &merged); err != nil {
				s.recordFailure(err)
				return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error()}
			}
		}
		return ApplyResult{Changed: false, Generation: s.store.Generation()}, nil
	}

	tmp, err := s.store.Prepare(merged)
	if err != nil {
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "persist_failed", Message: err.Error()}
	}
	committed := false
	defer func() {
		if !committed {
			s.store.Discard(tmp)
		}
	}()

	var oldProxy proxy.Proxy
	if proxyChanged {
		s.setApply(true, "stopping")
		s.mu.Lock()
		oldProxy = s.current
		s.current = nil
		s.proxyState = string(proxy.StateStarting)
		s.proxyErr = ""
		s.mu.Unlock()

		if oldProxy != nil {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = oldProxy.Stop(stopCtx)
			cancel()
		}

		s.setApply(true, "starting_proxy")
		newProxy, err := s.factory(&merged)
		if err == nil && newProxy == nil {
			err = errors.New("proxy factory returned nil")
		}
		if err == nil {
			err = newProxy.Start()
		}
		if err != nil {
			if newProxy != nil {
				stopProxy(newProxy)
			}
			s.setProxyError(err)
			rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
			s.recordFailure(err)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error(), RolledBack: rolledBack}
		}
		if !s.adoptProxy(newProxy) {
			s.recordFailure(context.Canceled)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
		}
	}

	if monitoringChanged {
		s.setApply(true, "starting_monitoring")
		s.stopMonitoring()
		if err := s.startMonitoring(&merged); err != nil {
			s.setMonitoringError(err)
			rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
			s.recordFailure(err)
			return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: err.Error(), RolledBack: rolledBack}
		}
	}

	if s.isShuttingDown() {
		s.recordFailure(context.Canceled)
		return ApplyResult{}, &ApplyError{Code: "apply_failed", Message: "server is shutting down"}
	}

	s.setApply(true, "committing")
	if err := s.store.Commit(tmp, merged); err != nil {
		rolledBack := s.rollback(current, proxyChanged, monitoringChanged)
		s.recordFailure(err)
		return ApplyResult{}, &ApplyError{Code: "persist_failed", Message: err.Error(), RolledBack: rolledBack}
	}
	committed = true

	now := time.Now()
	generation := s.store.Generation()
	s.mu.Lock()
	s.cfg = merged
	s.lastAppliedAt = now
	s.lastApply = LastApply{OK: true, At: now, Generation: generation}
	s.mu.Unlock()
	s.metrics.SetInterface(merged.Network.InterfaceName)

	return ApplyResult{
		Changed:        true,
		Generation:     generation,
		Restarted:      Restarted{Proxy: proxyChanged, Monitoring: monitoringChanged},
		ReauthRequired: reauthRequired,
	}, nil
}

func (s *Supervisor) Retry(ctx context.Context) error {
	if !s.applyMu.TryLock() {
		return ErrApplyInProgress
	}
	defer s.applyMu.Unlock()
	if s.isShuttingDown() {
		return errors.New("server is shutting down")
	}

	s.mu.Lock()
	p := s.current
	state := s.proxyState
	s.mu.Unlock()
	if p != nil && p.Status().State != proxy.StateError {
		return ErrProxyAlreadyRunning
	}
	if p == nil && (state == string(proxy.StateRunning) || state == string(proxy.StateStarting)) {
		return ErrProxyAlreadyRunning
	}

	s.setApply(true, "starting_proxy")
	defer s.setApply(false, "")
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if err := s.restartProxy(ctx, &cfg); err != nil {
		s.recordFailure(err)
		return err
	}
	return nil
}

func (s *Supervisor) restartProxy(ctx context.Context, cfg *config.ProxyConfig) error {
	s.mu.Lock()
	old := s.current
	s.current = nil
	s.mu.Unlock()
	if old != nil {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = old.Stop(stopCtx)
		cancel()
	}

	p, err := s.factory(cfg)
	if err == nil && p == nil {
		err = errors.New("proxy factory returned nil")
	}
	if err == nil {
		err = p.Start()
	}
	if err != nil {
		if p != nil {
			stopProxy(p)
		}
		s.setProxyError(err)
		return err
	}
	if !s.adoptProxy(p) {
		return context.Canceled
	}
	return nil
}

func (s *Supervisor) rollback(oldCfg config.ProxyConfig, proxyChanged, monitoringChanged bool) bool {
	ok := true
	if proxyChanged {
		s.mu.Lock()
		current := s.current
		s.current = nil
		s.mu.Unlock()
		if current != nil {
			stopProxy(current)
		}

		p, err := s.factory(&oldCfg)
		if err == nil && p == nil {
			err = errors.New("proxy factory returned nil")
		}
		if err == nil {
			err = p.Start()
		}
		if err != nil {
			if p != nil {
				stopProxy(p)
			}
			s.setProxyError(fmt.Errorf("rollback failed: %w", err))
			ok = false
		} else if !s.adoptProxy(p) {
			ok = false
		}
	}
	if monitoringChanged {
		s.stopMonitoring()
		if err := s.startMonitoring(&oldCfg); err != nil {
			s.mu.Lock()
			s.monitoringState = "error"
			s.monitoringErr = err.Error()
			s.mu.Unlock()
			ok = false
		}
	}
	return ok
}

func (s *Supervisor) Shutdown(ctx context.Context) {
	// Shutdown is idempotent: setting the flag repeatedly is harmless and a
	// second call finds no components left to stop.
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()

	// Let an in-flight apply observe the shutdown flag before stopping
	// components it may still be using. The wait is bounded by ctx.
	s.waitForApply(ctx)

	s.mu.Lock()
	p := s.current
	s.current = nil
	s.proxyState = string(proxy.StateStopped)
	srv := s.monitoring
	s.monitoring = nil
	s.monitoringState = "stopped"
	s.mu.Unlock()

	if p != nil {
		_ = p.Stop(ctx)
	}
	if srv != nil {
		_ = srv.Stop()
	}
}

func (s *Supervisor) waitForApply(ctx context.Context) {
	if s.applyMu.TryLock() {
		s.applyMu.Unlock()
		return
	}
	acquired := make(chan struct{})
	go func() {
		s.applyMu.Lock()
		defer s.applyMu.Unlock()
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-ctx.Done():
	}
}

func (s *Supervisor) isShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
}

func (s *Supervisor) Config() (config.ProxyConfig, uint64) {
	return s.store.Snapshot()
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	cfg := s.cfg
	p := s.current
	startedAt := s.startedAt
	proxyState := s.proxyState
	proxyErr := s.proxyErr
	monitoringState := s.monitoringState
	monitoringErr := s.monitoringErr
	lastAppliedAt := s.lastAppliedAt
	lastApply := s.lastApply
	applying := s.applyInProgress
	step := s.applyStep
	s.mu.Unlock()

	ps := ProxyStatus{State: proxyState, Type: cfg.GetProxyType(), Protocol: cfg.GetProxyProtocol(), Error: proxyErr}
	if p != nil {
		st := p.Status()
		ps = ProxyStatus{
			State:    string(st.State),
			Type:     st.Type,
			Listen:   st.Listen,
			Protocol: st.Protocol,
			Error:    st.Error,
		}
		if !st.StartedAt.IsZero() {
			t := st.StartedAt
			ps.StartedAt = &t
		}
	}
	if ps.State == "" {
		ps.State = string(proxy.StateStopped)
	}

	pool := PoolStatus{
		Enabled:       cfg.Network.RotateIPv6,
		Prefix:        cfg.Network.IPv6Subnet,
		MaxUsage:      cfg.Network.IPv6MaxUsage,
		MaxAgeMinutes: cfg.Network.IPv6MaxAge,
		Size:          cfg.Network.IPv6PoolSize,
	}
	if p != nil {
		if mgr := p.IPv6Manager(); mgr != nil {
			pool.Enabled = true
			pool.Size = mgr.GetPoolSize()
			pool.Prefix = mgr.Prefix()
			pool.Rotations = mgr.GetTotalRotated()
		}
	}

	stats := s.metrics.GetStats()
	now := time.Now()
	var uptime int64
	if !startedAt.IsZero() {
		uptime = int64(now.Sub(startedAt).Seconds())
	}
	status := Status{
		Version:       s.versionString(),
		UptimeSeconds: uptime,
		Proxy:         ps,
		Monitoring: MonitoringStatus{
			Enabled: cfg.Monitoring.Enabled,
			State:   monitoringState,
			Error:   monitoringErr,
		},
		Pool: pool,
		Metrics: MetricsStatus{
			ActiveConnections: stats.ActiveConnections,
			TotalConnections:  stats.TotalConnections,
			TotalRequests:     stats.TotalRequests,
			BytesSent:         stats.BytesSent,
			BytesReceived:     stats.BytesReceived,
		},
		Config: ConfigStatus{
			Path:       s.store.Path(),
			Generation: s.store.Generation(),
			LastApply:  lastApply,
		},
		Platform: PlatformStatus{
			OS:        runtime.GOOS,
			IPCommand: hasIPCommand(),
			Warnings:  []string{},
		},
		Apply: ApplyStatus{InProgress: applying, Step: step},
	}
	if !lastAppliedAt.IsZero() {
		t := lastAppliedAt
		status.Config.LastAppliedAt = &t
	}

	if !status.Platform.IPCommand {
		status.Platform.Warnings = append(status.Platform.Warnings, "the ip command was not found; IPv6 rotation is unavailable")
	}
	if !lastApply.OK && !lastApply.At.IsZero() {
		status.Platform.Warnings = append(status.Platform.Warnings, "last configuration apply failed: "+lastApply.Error)
	}
	if ps.State == string(proxy.StateError) {
		status.Platform.Warnings = append(status.Platform.Warnings, "proxy is not running: "+ps.Error)
	}
	return status
}

func (s *Supervisor) adoptProxy(p proxy.Proxy) bool {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		stopProxy(p)
		return false
	}
	s.current = p
	s.proxyState = string(proxy.StateRunning)
	s.proxyErr = ""
	s.mu.Unlock()
	go func() {
		err := p.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.current != p {
			return
		}
		if err != nil {
			s.proxyState = string(proxy.StateError)
			s.proxyErr = err.Error()
			return
		}
		s.proxyState = string(proxy.StateStopped)
	}()
	return true
}

func (s *Supervisor) startMonitoring(cfg *config.ProxyConfig) error {
	if !cfg.Monitoring.Enabled {
		s.mu.Lock()
		s.monitoring = nil
		s.monitoringState = "stopped"
		s.monitoringErr = ""
		s.mu.Unlock()
		return nil
	}

	srv := monitoring.New(cfg, s.metrics, version.GetVersionInfo())
	if err := srv.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	s.monitoring = srv
	s.monitoringState = "running"
	s.monitoringErr = ""
	s.mu.Unlock()
	return nil
}

func (s *Supervisor) stopMonitoring() {
	s.mu.Lock()
	srv := s.monitoring
	s.monitoring = nil
	s.monitoringState = "stopped"
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Stop()
	}
}

func (s *Supervisor) setProxyError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proxyState = string(proxy.StateError)
	s.proxyErr = err.Error()
}

func (s *Supervisor) setMonitoringError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.monitoringState = "error"
	s.monitoringErr = err.Error()
}

func (s *Supervisor) setApply(inProgress bool, step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyInProgress = inProgress
	s.applyStep = step
}

func (s *Supervisor) recordFailure(err error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastApply = LastApply{OK: false, Error: err.Error(), At: now, Generation: s.store.Generation()}
}

func (s *Supervisor) versionString() string {
	if s.buildVersion != "" {
		return s.buildVersion
	}
	if version.Commit != "" && version.Commit != "unknown" {
		return version.Commit
	}
	return version.Version
}

func validationFields(err error) map[string]string {
	var verr *config.ValidationError
	if errors.As(err, &verr) {
		return verr.Fields
	}
	return map[string]string{}
}

func hasIPCommand() bool {
	_, err := exec.LookPath("ip")
	return err == nil
}

func stopProxy(p proxy.Proxy) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.Stop(ctx)
}
```

Remove the `net` import and `var _ = net.IPv4len`. The supervisor does not use `net`.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/app/... -count=1
go test -race ./internal/app/... -count=1
go vet ./internal/app/...
```

Expected: PASS. If the rollback test reports `RolledBack: false`, inspect that `rollback` started the third fake proxy successfully and `sup.current` was restored.

- [ ] **Step 7: Commit checkpoint**

```bash
git add internal/app
git commit -m "feat(app): add transactional supervisor with rollback and retry"
```

Only run if the user has authorized commits.

## Task 7: API Foundation, Authentication, Sessions, and CSRF

**Files:**
- Create: `internal/api/errors.go`
- Create: `internal/api/auth.go`
- Create: `internal/api/server.go`
- Create: `internal/api/auth_test.go`
- Create: `internal/api/server_test.go`

**Interfaces:**
- Consumes: `config.ProxyConfig`, `config.Store`, `app.Status`, `app.ApplyResult`, `logbuf.Broker`
- Produces:
  - `type Controller interface { Status() app.Status; Config() (config.ProxyConfig, uint64); Apply(context.Context, config.ProxyConfig) (app.ApplyResult, error); Retry(context.Context) error }`
  - `type Options struct { Config config.ProxyConfig; Store *config.Store; Controller Controller; Auth *Auth; Logs *logbuf.Broker; UI http.Handler }`
  - `func New(opts Options) *Server`
  - `func (s *Server) Start() error`
  - `func (s *Server) Wait() error`
  - `func (s *Server) Shutdown(ctx context.Context) error`
  - `func (s *Server) Addr() string`
  - `func (s *Server) Handler() http.Handler`
  - `func NewAuth(username, password string, opts AuthOptions) *Auth`
  - `type AuthOptions struct { CookieSecure bool; TrustProxyHeaders bool; Now func() time.Time; Sleep func(time.Duration) }`
  - `func (a *Auth) Login(w http.ResponseWriter, r *http.Request, username, password string) (*Session, error)`
  - `func (a *Auth) SessionFromRequest(r *http.Request) *Session`
  - `func (a *Auth) SessionByID(id string) *Session`
  - `func (a *Auth) CSRFValid(s *Session, token string) bool`
  - `func (a *Auth) Logout(id string)`
  - `func (a *Auth) SetCredentials(username, password string)`
  - `func (a *Auth) InvalidateAll()`
  - `func (a *Auth) CheckOrigin(r *http.Request) error`
  - `func (a *Auth) ClearCookie(w http.ResponseWriter, r *http.Request)`
  - `var ErrInvalidCredentials`, `var ErrOriginMismatch`, `var ErrUnauthenticated`
  - `type RateLimitedError struct { RetryAfter time.Duration }`
  - `func writeJSON(w http.ResponseWriter, status int, v any)`
  - `func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string)`

- [ ] **Step 1: Write failing auth tests**

Create `internal/api/auth_test.go`:

```go
package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestAuth(now func() time.Time) *Auth {
	return NewAuth("admin", "secret", AuthOptions{
		Now:   now,
		Sleep: func(time.Duration) {},
	})
}

func loginRequest(t *testing.T, a *Auth, username, password string) (*httptest.ResponseRecorder, *Session) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.Host = "example.com"
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()
	session, err := a.Login(rr, req, username, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return rr, session
}

func TestLoginCreatesSessionAndCookie(t *testing.T) {
	a := newTestAuth(time.Now)
	rr, session := loginRequest(t, a, "admin", "secret")
	if session.CSRF == "" || session.ID == "" {
		t.Fatalf("session = %+v", session)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie {
		t.Fatalf("cookies = %#v", cookies)
	}
	if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags = %+v", cookies[0])
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	a := newTestAuth(time.Now)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()

	_, err := a.Login(rr, req, "admin", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginRateLimitAfterFiveFailures(t *testing.T) {
	a := newTestAuth(time.Now)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		_, _ = a.Login(httptest.NewRecorder(), req, "admin", "wrong")
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	_, err := a.Login(httptest.NewRecorder(), req, "admin", "secret")
	var rateErr *RateLimitedError
	if !errors.As(err, &rateErr) {
		t.Fatalf("error = %v, want RateLimitedError", err)
	}
	if rateErr.RetryAfter <= 0 {
		t.Fatalf("retry after = %v", rateErr.RetryAfter)
	}
}

func TestSessionExpiresAfterTwelveHours(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	a := newTestAuth(func() time.Time { return now })
	_, session := loginRequest(t, a, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.ID})
	if got := a.SessionFromRequest(req); got == nil {
		t.Fatal("session should be valid")
	}

	now = now.Add(12*time.Hour + time.Second)
	if got := a.SessionFromRequest(req); got != nil {
		t.Fatal("session should be expired")
	}
}

func TestCSRFComparison(t *testing.T) {
	a := newTestAuth(time.Now)
	_, session := loginRequest(t, a, "admin", "secret")
	if !a.CSRFValid(session, session.CSRF) {
		t.Fatal("expected CSRF token to match")
	}
	if a.CSRFValid(session, "wrong") {
		t.Fatal("expected mismatch")
	}
}

func TestOriginMustMatchHost(t *testing.T) {
	a := newTestAuth(time.Now)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.Host = "example.com"
	req.Header.Set("Origin", "https://evil.example")
	if !errors.Is(a.CheckOrigin(req), ErrOriginMismatch) {
		t.Fatal("expected origin mismatch")
	}
	req.Header.Set("Origin", "https://example.com")
	if err := a.CheckOrigin(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetCredentialsInvalidatesSessions(t *testing.T) {
	a := newTestAuth(time.Now)
	_, session := loginRequest(t, a, "admin", "secret")
	a.SetCredentials("admin", "new-secret")
	if a.SessionByID(session.ID) != nil {
		t.Fatal("session should be invalidated")
	}
	if a.SessionByID(session.ID) != nil {
		t.Fatal("session should stay invalidated")
	}
}
```

Create `internal/api/server_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
)

type fakeController struct {
	status app.Status
	cfg    config.ProxyConfig
	gen    uint64
}

func (f *fakeController) Status() app.Status              { return f.status }
func (f *fakeController) Config() (config.ProxyConfig, uint64) { return f.cfg, f.gen }
func (f *fakeController) Apply(ctx context.Context, cfg config.ProxyConfig) (app.ApplyResult, error) {
	return app.ApplyResult{}, nil
}
func (f *fakeController) Retry(ctx context.Context) error { return nil }

func testServer(t *testing.T) (*Server, *Auth) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
proxy:
  type: socks5
  listen_address: 127.0.0.1
  listen_port: 1080
  username: admin
  password: secret
network:
  ipv4_enabled: true
  ipv6_enabled: false
monitoring:
  enabled: false
logging:
  debug_level: 1
web:
  enabled: true
  listen_address: 127.0.0.1
  port: 18081
  username: admin
  password: web-secret
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := store.Snapshot()
	auth := NewAuth(cfg.Web.Username, cfg.Web.Password, AuthOptions{
		Now:   time.Now,
		Sleep: func(time.Duration) {},
	})
	ctrl := &fakeController{cfg: cfg, gen: 1}
	srv := New(Options{
		Config:     cfg,
		Store:      store,
		Controller: ctrl,
		Auth:       auth,
		Logs:       logbuf.New(10, 2),
		UI:         http.NotFoundHandler(),
	})
	return srv, auth
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security headers")
	}
}

func TestStatusRequiresSession(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "unauthenticated" {
		t.Fatalf("code = %q", envelope.Error.Code)
	}
}

func TestLoginSessionAndLogoutFlow(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.Handler()

	loginBody := `{"username":"admin","password":"web-secret"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", strings.NewReader(loginBody))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rr.Code, rr.Body.String())
	}
	cookie := rr.Result().Cookies()[0]

	var login struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/auth/session", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/logout", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", login.CSRFToken)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", rr.Code, rr.Body.String())
	}
}
```

Add `strings` to the imports. Do not leave it out.

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./internal/api/... -count=1
```

Expected: compile errors for `Auth`, `New`, `Server`, `Options`, error types, and `writeJSON`.

- [ ] **Step 3: Implement API errors and JSON helpers**

Create `internal/api/errors.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

type apiError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Fields     map[string]string `json:"fields,omitempty"`
	RolledBack *bool             `json:"rolled_back,omitempty"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, Fields: fields}})
}
```

- [ ] **Step 4: Implement authentication**

Create `internal/api/auth.go`:

```go
package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "nyxproxy_session"

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrOriginMismatch = errors.New("origin does not match host")
var ErrUnauthenticated = errors.New("unauthenticated")

type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string { return "too many login attempts" }

type Session struct {
	ID       string
	CSRF     string
	Username string
	Created  time.Time
	Expires  time.Time
}

type AuthOptions struct {
	CookieSecure      bool
	TrustProxyHeaders bool
	Now               func() time.Time
	Sleep             func(time.Duration)
}

type failureWindow struct {
	count int
	first time.Time
}

type Auth struct {
	mu       sync.Mutex
	username string
	password string
	sessions map[string]*Session
	failures map[string]*failureWindow

	cookieSecure      bool
	trustProxyHeaders bool
	now               func() time.Time
	sleep             func(time.Duration)

	sessionTTL  time.Duration
	maxSessions int
}

func NewAuth(username, password string, opts AuthOptions) *Auth {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Sleep == nil {
		opts.Sleep = time.Sleep
	}
	return &Auth{
		username:          username,
		password:          password,
		sessions:          make(map[string]*Session),
		failures:          make(map[string]*failureWindow),
		cookieSecure:      opts.CookieSecure,
		trustProxyHeaders: opts.TrustProxyHeaders,
		now:               opts.Now,
		sleep:             opts.Sleep,
		sessionTTL:        12 * time.Hour,
		maxSessions:       100,
	}
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request, username, password string) (*Session, error) {
	ip := a.ClientIP(r)

	a.mu.Lock()
	if retry, ok := a.rateLimitedLocked(ip); ok {
		a.mu.Unlock()
		return nil, &RateLimitedError{RetryAfter: retry}
	}

	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
	if !userOK || !passOK {
		a.recordFailureLocked(ip)
		a.mu.Unlock()
		a.sleep(200 * time.Millisecond)
		return nil, ErrInvalidCredentials
	}

	now := a.now()
	a.cleanupLocked(now)
	for len(a.sessions) >= a.maxSessions {
		var oldestID string
		var oldest time.Time
		for id, s := range a.sessions {
			if oldestID == "" || s.Created.Before(oldest) {
				oldestID = id
				oldest = s.Created
			}
		}
		delete(a.sessions, oldestID)
	}

	session := &Session{
		ID:       newToken(),
		CSRF:     newToken(),
		Username: username,
		Created:  now,
		Expires:  now.Add(a.sessionTTL),
	}
	a.sessions[session.ID] = session
	delete(a.failures, ip)
	a.mu.Unlock()

	a.setCookie(w, r, session)
	return session, nil
}

func (a *Auth) SessionFromRequest(r *http.Request) *Session {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	return a.SessionByID(cookie.Value)
}

func (a *Auth) SessionByID(id string) *Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[id]
	if !ok {
		return nil
	}
	if a.now().After(session.Expires) {
		delete(a.sessions, id)
		return nil
	}
	return session
}

func (a *Auth) CSRFValid(session *Session, token string) bool {
	if session == nil || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(session.CSRF)) == 1
}

func (a *Auth) Logout(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, id)
}

func (a *Auth) SetCredentials(username, password string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.username = username
	a.password = password
	a.sessions = make(map[string]*Session)
}

func (a *Auth) InvalidateAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions = make(map[string]*Session)
}

func (a *Auth) CheckOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
		return ErrOriginMismatch
	}
	return nil
}

func (a *Auth) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.secure(r),
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

func (a *Auth) StartCleanup() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			a.mu.Lock()
			a.cleanupLocked(a.now())
			a.mu.Unlock()
		}
	}()
}

func (a *Auth) ClientIP(r *http.Request) string {
	if a.trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *Auth) secure(r *http.Request) bool {
	if a.cookieSecure || r.TLS != nil {
		return true
	}
	if a.trustProxyHeaders {
		return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
	return false
}

func (a *Auth) setCookie(w http.ResponseWriter, r *http.Request, session *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.secure(r),
		Expires:  session.Expires,
		MaxAge:   int(a.sessionTTL.Seconds()),
	})
}

func (a *Auth) rateLimitedLocked(ip string) (time.Duration, bool) {
	window, ok := a.failures[ip]
	if !ok {
		return 0, false
	}
	now := a.now()
	if now.Sub(window.first) >= 5*time.Minute {
		delete(a.failures, ip)
		return 0, false
	}
	if window.count < 5 {
		return 0, false
	}
	return 5*time.Minute - now.Sub(window.first), true
}

func (a *Auth) recordFailureLocked(ip string) {
	now := a.now()
	window, ok := a.failures[ip]
	if !ok || now.Sub(window.first) >= 5*time.Minute {
		a.failures[ip] = &failureWindow{count: 1, first: now}
		return
	}
	window.count++
}

func (a *Auth) cleanupLocked(now time.Time) {
	for id, session := range a.sessions {
		if now.After(session.Expires) {
			delete(a.sessions, id)
		}
	}
	for ip, window := range a.failures {
		if now.Sub(window.first) >= 5*time.Minute {
			delete(a.failures, ip)
		}
	}
}

func newToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
```

- [ ] **Step 5: Implement the server shell**

Create `internal/api/server.go`:

```go
package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
)

type Controller interface {
	Status() app.Status
	Config() (config.ProxyConfig, uint64)
	Apply(ctx context.Context, cfg config.ProxyConfig) (app.ApplyResult, error)
	Retry(ctx context.Context) error
}

type Options struct {
	Config     config.ProxyConfig
	Store      *config.Store
	Controller Controller
	Auth       *Auth
	Logs       *logbuf.Broker
	UI         http.Handler
}

type Server struct {
	opts Options
	auth *Auth

	httpServer *http.Server
	mu         sync.Mutex
	listener   net.Listener
	serveErr   error
	done       chan struct{}
}

func New(opts Options) *Server {
	if opts.Auth == nil {
		opts.Auth = NewAuth(opts.Config.Web.Username, opts.Config.Web.Password, AuthOptions{
			CookieSecure:      opts.Config.Web.CookieSecure,
			TrustProxyHeaders: opts.Config.Web.TrustProxyHeaders,
		})
	}
	if opts.UI == nil {
		opts.UI = http.NotFoundHandler()
	}
	s := &Server{opts: opts, auth: opts.Auth, done: make(chan struct{})}
	s.httpServer = &http.Server{
		Handler:           s.securityHeaders(s.accessLog(s.routes())),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/auth/session", s.requireSession(s.handleSession))
	mux.HandleFunc("POST /api/auth/logout", s.requireSession(s.requireCSRF(s.handleLogout)))
	mux.HandleFunc("/api/", s.requireSession(s.handleAPINotFound))
	mux.Handle("/", s.opts.UI)
	return mux
}

func (s *Server) Start() error {
	addr := net.JoinHostPort(s.opts.Config.Web.ListenAddress, strconv.Itoa(s.opts.Config.Web.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("web console listen: %w", err)
	}

	if s.opts.Config.Web.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(s.opts.Config.Web.TLSCertFile, s.opts.Config.Web.TLSKeyFile)
		if err != nil {
			ln.Close()
			return fmt.Errorf("load tls certificate: %w", err)
		}
		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.auth.StartCleanup()
	log.Printf("web console listening on %s (tls=%v)", ln.Addr().String(), s.opts.Config.Web.TLSCertFile != "")

	go func() {
		err := s.httpServer.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		s.mu.Lock()
		s.serveErr = err
		close(s.done)
		s.mu.Unlock()
	}()
	return nil
}

func (s *Server) Wait() error {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serveErr
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.CheckOrigin(r); err != nil {
		writeError(w, http.StatusForbidden, "origin_mismatch", "origin does not match host", nil)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err.status, err.code, err.message, nil)
		return
	}
	session, err := s.auth.Login(w, r, body.Username, body.Password)
	if err != nil {
		var rate *RateLimitedError
		switch {
		case errors.As(err, &rate):
			seconds := int(rate.RetryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts", nil)
		default:
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   session.Username,
		"csrf_token": session.CSRF,
		"expires_at": session.Expires.UTC(),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request, session *Session) {
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   session.Username,
		"csrf_token": session.CSRF,
		"expires_at": session.Expires.UTC(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, session *Session) {
	s.auth.Logout(session.ID)
	s.auth.ClearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request, session *Session) {
	writeError(w, http.StatusNotFound, "not_found", "not found", nil)
}

type sessionHandler func(w http.ResponseWriter, r *http.Request, session *Session)

func (s *Server) requireSession(next sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.auth.SessionFromRequest(r)
		if session == nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication required", nil)
			return
		}
		next(w, r, session)
	}
}

func (s *Server) requireCSRF(next sessionHandler) sessionHandler {
	return func(w http.ResponseWriter, r *http.Request, session *Session) {
		if err := s.auth.CheckOrigin(r); err != nil {
			writeError(w, http.StatusForbidden, "origin_mismatch", "origin does not match host", nil)
			return
		}
		if !s.auth.CSRFValid(session, r.Header.Get("X-CSRF-Token")) {
			writeError(w, http.StatusForbidden, "csrf_failed", "invalid CSRF token", nil)
			return
		}
		next(w, r, session)
	}
}

type decodeError struct {
	status  int
	code    string
	message string
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) *decodeError {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return &decodeError{http.StatusUnsupportedMediaType, "unsupported_media_type", "content type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return &decodeError{http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 1 MiB"}
		}
		return &decodeError{http.StatusBadRequest, "invalid_json", err.Error()}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return &decodeError{http.StatusBadRequest, "invalid_json", "request body must contain one JSON object"}
	}
	return nil
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil || (s.opts.Config.Web.TrustProxyHeaders && r.Header.Get("X-Forwarded-Proto") == "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

var accessLog = log.New(os.Stderr, "", log.LstdFlags)

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		accessLog.Printf("%s %s %s %s", s.auth.ClientIP(r), r.Method, r.URL.Path, time.Since(start))
	})
}
```

Add `strings` and `encoding/json` imports. Remove the unused `handlerContextKey` type. Also remove unused `sessionHandler` alias `csrfHandler`. Keep only types used. `writeJSON` is used.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/api/... -count=1
go vet ./internal/api/...
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

```bash
git add internal/api
git commit -m "feat(api): add session auth, CSRF, origin checks, and security headers"
```

Only run if the user has authorized commits.

## Task 8: Configuration, Status, Retry, Interfaces, SSE, and Embedded UI

**Files:**
- Create: `internal/api/handlers.go`
- Create: `internal/api/handlers_test.go`
- Modify: `internal/api/server.go`
- Create: `internal/webui/embed.go`
- Create: `internal/webui/embed_test.go`
- Create: `internal/webui/dist/.gitkeep`

**Interfaces:**
- Consumes: `Controller`, `config.LockedFields`, `config.ProxyConfig.Redacted`, `config.ProxyConfig.PasswordsSet`, `network.ListInterfaceDetails`, `logbuf.Broker`
- Produces:
  - `func Handler() http.Handler` in `internal/webui`
  - `func handlerFor(fsys fs.FS) http.Handler` (unexported, used by tests)
  - HTTP handlers for `/api/status`, `/api/config`, `/api/proxy/retry`, `/api/network/interfaces`, `/api/logs/stream`

- [ ] **Step 1: Write failing handler tests**

Create `internal/api/handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
)

func loginCookieAndCSRF(t *testing.T, srv *Server) (*http.Cookie, string) {
	t.Helper()
	body := `{"username":"admin","password":"web-secret"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", strings.NewReader(body))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return rr.Result().Cookies()[0], payload.CSRF
}

func TestGetConfigRedactsPasswords(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/config", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Config config.ProxyConfig `json:"config"`
		Meta   struct {
			Path         string          `json:"path"`
			Generation   uint64          `json:"generation"`
			Locked       map[string]string `json:"locked"`
			PasswordsSet map[string]bool `json:"passwords_set"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Config.Proxy.Password != "" || payload.Config.Web.Password != "" {
		t.Fatalf("passwords were not redacted: %+v", payload.Config)
	}
	if !payload.Meta.PasswordsSet["proxy.password"] || !payload.Meta.PasswordsSet["web.password"] {
		t.Fatalf("passwords_set = %#v", payload.Meta.PasswordsSet)
	}
	if payload.Meta.Generation != 1 {
		t.Fatalf("generation = %d", payload.Meta.Generation)
	}
}

func TestPutConfigReturnsAppliedResult(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.applyResult = app.ApplyResult{Changed: true, Generation: 2, Restarted: app.Restarted{Proxy: true}}
	cookie, csrf := loginCookieAndCSRF(t, srv)

	cfg, _ := ctrl.Config()
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result app.ApplyResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Generation != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestPutConfigRequiresCSRF(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(`{}`))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
	var envelope errorEnvelope
	json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.Error.Code != "csrf_failed" {
		t.Fatalf("code = %q", envelope.Error.Code)
	}
}

func TestPutConfigMapsLockedError(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.applyErr = &app.ApplyError{
		Code:    "restart_required",
		Message: "changed fields require a restart",
		Fields:  map[string]string{"web.port": "requires restart"},
	}
	cookie, csrf := loginCookieAndCSRF(t, srv)
	cfg, _ := ctrl.Config()
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/config", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope errorEnvelope
	json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.Error.Code != "restart_required" || envelope.Error.Fields["web.port"] == "" {
		t.Fatalf("error = %+v", envelope.Error)
	}
}

func TestInterfacesReturnsJSONArray(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/network/interfaces", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var details []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &details); err != nil {
		t.Fatalf("response is not a JSON array: %v", err)
	}
}

func TestStatusReturnsControllerStatus(t *testing.T) {
	srv, _ := testServer(t)
	ctrl := srv.opts.Controller.(*fakeController)
	ctrl.status = app.Status{Version: "test", Proxy: app.ProxyStatus{State: "running"}}
	cookie, _ := loginCookieAndCSRF(t, srv)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var status app.Status
	json.Unmarshal(rr.Body.Bytes(), &status)
	if status.Proxy.State != "running" {
		t.Fatalf("status = %+v", status.Proxy)
	}
}

func TestLogsStreamReplaysBufferedRecords(t *testing.T) {
	srv, _ := testServer(t)
	cookie, _ := loginCookieAndCSRF(t, srv)
	srv.opts.Logs.Write([]byte("hello from the proxy\n"))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs/stream", nil).WithContext(ctx)
	req.Host = "example.com"
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not stop")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "hello from the proxy") {
		t.Fatalf("body = %q", body)
	}
	if rr.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("missing X-Accel-Buffering")
	}
}
```

Update `fakeController` in `server_test.go`:

```go
type fakeController struct {
	status      app.Status
	cfg         config.ProxyConfig
	gen         uint64
	applyResult app.ApplyResult
	applyErr    error
	retryErr    error
}

func (f *fakeController) Status() app.Status                    { return f.status }
func (f *fakeController) Config() (config.ProxyConfig, uint64)  { return f.cfg, f.gen }
func (f *fakeController) Apply(ctx context.Context, cfg config.ProxyConfig) (app.ApplyResult, error) {
	return f.applyResult, f.applyErr
}
func (f *fakeController) Retry(ctx context.Context) error { return f.retryErr }
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./internal/api/... ./internal/webui/... -count=1
```

Expected: compile errors for the missing routes, handlers, and `webui` package.

- [ ] **Step 3: Add the new API routes**

In `internal/api/server.go`, replace `routes()` with:

```go
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/auth/session", s.requireSession(s.handleSession))
	mux.HandleFunc("POST /api/auth/logout", s.requireSession(s.requireCSRF(s.handleLogout)))

	mux.HandleFunc("GET /api/status", s.requireSession(s.handleStatus))
	mux.HandleFunc("GET /api/config", s.requireSession(s.handleGetConfig))
	mux.HandleFunc("PUT /api/config", s.requireSession(s.requireCSRF(s.handlePutConfig)))
	mux.HandleFunc("POST /api/proxy/retry", s.requireSession(s.requireCSRF(s.handleRetry)))
	mux.HandleFunc("GET /api/network/interfaces", s.requireSession(s.handleInterfaces))
	mux.HandleFunc("GET /api/logs/stream", s.requireSession(s.handleLogs))

	mux.HandleFunc("/healthz", s.methodNotAllowed)
	mux.HandleFunc("/api/auth/login", s.methodNotAllowed)
	mux.HandleFunc("/api/auth/session", s.methodNotAllowed)
	mux.HandleFunc("/api/auth/logout", s.methodNotAllowed)
	mux.HandleFunc("/api/status", s.methodNotAllowed)
	mux.HandleFunc("/api/config", s.methodNotAllowed)
	mux.HandleFunc("/api/proxy/retry", s.methodNotAllowed)
	mux.HandleFunc("/api/network/interfaces", s.methodNotAllowed)
	mux.HandleFunc("/api/logs/stream", s.methodNotAllowed)

	mux.HandleFunc("/api/", s.requireSession(s.handleAPINotFound))
	mux.Handle("/", s.opts.UI)
	return mux
}

func (s *Server) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
}
```

- [ ] **Step 4: Implement the API handlers**

Create `internal/api/handlers.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, session *Session) {
	writeJSON(w, http.StatusOK, s.opts.Controller.Status())
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request, session *Session) {
	cfg, generation := s.opts.Controller.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"config": cfg.Redacted(),
		"meta": map[string]any{
			"path":          s.opts.Store.Path(),
			"generation":    generation,
			"locked":        config.LockedFields(),
			"passwords_set": cfg.PasswordsSet(),
		},
	})
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request, session *Session) {
	var submitted config.ProxyConfig
	if derr := decodeJSON(w, r, &submitted); derr != nil {
		writeError(w, derr.status, derr.code, derr.message, nil)
		return
	}

	result, err := s.opts.Controller.Apply(r.Context(), submitted)
	if err != nil {
		var applyErr *app.ApplyError
		switch {
		case errors.Is(err, app.ErrApplyInProgress):
			writeError(w, http.StatusConflict, "apply_in_progress", "a configuration apply is already in progress", nil)
		case errors.As(err, &applyErr):
			status := http.StatusInternalServerError
			switch applyErr.Code {
			case "validation_failed", "field_locked", "restart_required":
				status = http.StatusUnprocessableEntity
			case "apply_in_progress":
				status = http.StatusConflict
			}
			var rolledBack *bool
			if applyErr.Code == "apply_failed" || applyErr.Code == "persist_failed" {
				rolledBack = &applyErr.RolledBack
			}
			writeJSON(w, status, errorEnvelope{Error: apiError{
				Code:       applyErr.Code,
				Message:    applyErr.Message,
				Fields:     applyErr.Fields,
				RolledBack: rolledBack,
			}})
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	if result.ReauthRequired {
		cfg, _ := s.opts.Controller.Config()
		s.auth.SetCredentials(cfg.Web.Username, cfg.Web.Password)
		s.auth.ClearCookie(w, r)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request, session *Session) {
	err := s.opts.Controller.Retry(r.Context())
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, app.ErrApplyInProgress):
		writeError(w, http.StatusConflict, "apply_in_progress", "a configuration apply is already in progress", nil)
	case errors.Is(err, app.ErrProxyAlreadyRunning):
		writeError(w, http.StatusConflict, "proxy_already_running", "proxy is already running", nil)
	default:
		writeError(w, http.StatusInternalServerError, "apply_failed", err.Error(), nil)
	}
}

func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request, session *Session) {
	details, err := network.ListInterfaceDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request, session *Session) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming is not supported", nil)
		return
	}
	if s.opts.Logs == nil {
		writeError(w, http.StatusServiceUnavailable, "internal_error", "log stream unavailable", nil)
		return
	}

	ch, cancel, ok := s.opts.Logs.Subscribe()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "internal_error", "too many log streams", nil)
		return
	}
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var after uint64
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if value, err := strconv.ParseUint(raw, 10, 64); err == nil {
			after = value
		}
	}
	if after > s.opts.Logs.LastSeq() {
		// The ID belongs to a previous process; sequence numbers restarted at 1.
		after = 0
	}

	records, dropped := s.opts.Logs.Replay(after)
	lastSeq := after
	if dropped > 0 {
		fmt.Fprintf(w, "event: gap\ndata: {\"dropped\":%d}\n\n", dropped)
	}
	for _, record := range records {
		writeLogEvent(w, record)
		lastSeq = record.Seq
	}
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	maxLifetime := time.NewTimer(30 * time.Minute)
	defer maxLifetime.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-maxLifetime.C:
			return
		case <-heartbeat.C:
			if s.auth.SessionByID(session.ID) == nil {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case record, open := <-ch:
			if !open {
				return
			}
			if record.Seq <= lastSeq {
				continue
			}
			writeLogEvent(w, record)
			lastSeq = record.Seq
			flusher.Flush()
		}
	}
}

func writeLogEvent(w http.ResponseWriter, record logbuf.Record) {
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: log\nid: %d\ndata: %s\n\n", record.Seq, data)
}
```

- [ ] **Step 5: Implement the embedded UI handler**

Create `internal/webui/dist/.gitkeep` (empty file) and `internal/webui/embed.go`:

```go
package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.HandlerFunc(placeholder)
	}
	return handlerFor(sub)
}

func handlerFor(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "." || clean == "" {
			clean = "index.html"
		}

		candidates := []string{clean}
		if path.Ext(clean) == "" {
			if strings.HasSuffix(r.URL.Path, "/") || clean == "index.html" {
				candidates = append(candidates, path.Join(clean, "index.html"))
			}
			candidates = append(candidates, clean+"/index.html", clean+".html")
		}

		for _, candidate := range candidates {
			data, err := fs.ReadFile(fsys, candidate)
			if err != nil {
				continue
			}
			contentType := mime.TypeByExtension(path.Ext(candidate))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			w.Header().Set("Content-Type", contentType)
			if strings.HasPrefix(r.URL.Path, "/_next/static/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-store")
			}
			_, _ = w.Write(data)
			return
		}

		if index, err := fs.ReadFile(fsys, "index.html"); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(index)
			return
		}
		placeholder(w, r)
	})
}

func placeholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>NyxProxy</title></head>
<body style="font-family: sans-serif; padding: 2rem">
<h1>NyxProxy UI not built</h1>
<p>Run <code>scripts/build-web.sh</code> and rebuild the Go binary.</p>
</body>
</html>`))
}
```

Create `internal/webui/embed_test.go`:

```go
package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPlaceholderWhenIndexMissing(t *testing.T) {
	handler := handlerFor(fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "UI not built") {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestServesRouteIndexAndStaticCache(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("root")},
		"config/index.html":  {Data: []byte("config-page")},
		"_next/static/a.js":  {Data: []byte("console.log(1)")},
	}
	handler := handlerFor(fsys)

	req := httptest.NewRequest(http.MethodGet, "/config/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Body.String() != "config-page" {
		t.Fatalf("body = %q", rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", rr.Header().Get("Cache-Control"))
	}

	req = httptest.NewRequest(http.MethodGet, "/_next/static/a.js", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Body.String() != "console.log(1)" {
		t.Fatalf("body = %q", rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("cache = %q", rr.Header().Get("Cache-Control"))
	}
}
```

The `candidates` logic for `/config/` produces `config` and `config/index.html`, so it serves the page. Good.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/api/... ./internal/webui/... -count=1
go vet ./internal/api/... ./internal/webui/...
```

Expected: PASS.

- [ ] **Step 7: Commit checkpoint**

```bash
git add internal/api internal/webui
git commit -m "feat(api): add config, status, retry, interfaces, SSE, and embedded UI"
```

Only run if the user has authorized commits.

## Task 9: Main Wiring, Config Path Precedence, and Healthcheck

**Files:**
- Modify: `cmd/proxy/main.go`
- Create: `cmd/proxy/main_test.go`

**Interfaces:**
- Consumes: `config.LoadConfigFrom`, `config.NewStore`, `app.New`, `app.DefaultFactory`, `api.New`, `webui.Handler`, `logbuf.New`
- Produces:
  - `func resolveConfigPath(flagValue string) string`
  - `func healthcheckURL(cfg *config.ProxyConfig) string`
  - `func runHealthcheck(path string) int`

- [ ] **Step 1: Write failing tests for path precedence and healthcheck URL**

Create `cmd/proxy/main_test.go`:

```go
package main

import (
	"testing"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
)

func TestResolveConfigPathPrecedence(t *testing.T) {
	t.Setenv("NYXPROXY_CONFIG", "/env/config.yaml")
	if got := resolveConfigPath("/flag/config.yaml"); got != "/flag/config.yaml" {
		t.Fatalf("flag path = %q", got)
	}
	if got := resolveConfigPath(""); got != "/env/config.yaml" {
		t.Fatalf("env path = %q", got)
	}
	t.Setenv("NYXPROXY_CONFIG", "")
	if got := resolveConfigPath(""); got != "config.yaml" {
		t.Fatalf("default path = %q", got)
	}
}

func TestHealthcheckURLUsesLoopbackForWildcard(t *testing.T) {
	cfg := &config.ProxyConfig{}
	cfg.Web.Enabled = true
	cfg.Web.ListenAddress = "0.0.0.0"
	cfg.Web.Port = 18081
	if got := healthcheckURL(cfg); got != "http://127.0.0.1:18081/healthz" {
		t.Fatalf("url = %q", got)
	}

	cfg.Web.TLSCertFile = "/certs/tls.crt"
	if got := healthcheckURL(cfg); got != "https://127.0.0.1:18081/healthz" {
		t.Fatalf("tls url = %q", got)
	}

	cfg.Web.ListenAddress = "localhost"
	if got := healthcheckURL(cfg); got != "https://localhost:18081/healthz" {
		t.Fatalf("localhost url = %q", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

```bash
go test ./cmd/proxy/... -count=1
```

Expected: compile errors for `resolveConfigPath` and `healthcheckURL`.

- [ ] **Step 3: Rewrite `main.go`**

Replace `cmd/proxy/main.go` with:

```go
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/api"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/webui"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/version"
)

func main() {
	showVersion := flag.Bool("version", false, "show version information")
	configFlag := flag.String("config", "", "path to config.yaml")
	healthcheck := flag.Bool("healthcheck", false, "check the web console health endpoint")
	flag.Parse()

	if *showVersion {
		fmt.Println("NyxProxy Core", version.GetVersionInfo())
		return
	}

	configPath := resolveConfigPath(*configFlag)
	if *healthcheck {
		os.Exit(runHealthcheck(configPath))
	}

	cfg, err := config.LoadConfigFrom(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if dir := filepath.Dir(configPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0700)
	}

	store, err := config.NewStore(configPath)
	if err != nil {
		log.Fatalf("Failed to initialize configuration store: %v", err)
	}

	broker := logbuf.New(2000, 8)
	log.SetOutput(io.MultiWriter(os.Stderr, broker))
	log.SetFlags(log.LstdFlags)

	interfaceName := cfg.Network.InterfaceName
	if interfaceName == "" {
		interfaceName = "auto-detect"
	}
	metricsCollector := metrics.New(interfaceName)
	supervisor := app.New(*cfg, store, app.DefaultFactory, metricsCollector, version.Commit)

	var apiServer *api.Server
	if cfg.Web.Enabled {
		auth := api.NewAuth(cfg.Web.Username, cfg.Web.Password, api.AuthOptions{
			CookieSecure:      cfg.Web.CookieSecure,
			TrustProxyHeaders: cfg.Web.TrustProxyHeaders,
		})
		apiServer = api.New(api.Options{
			Config:     *cfg,
			Store:      store,
			Controller: supervisor,
			Auth:       auth,
			Logs:       broker,
			UI:         webui.Handler(),
		})
		if err := apiServer.Start(); err != nil {
			log.Fatalf("Failed to start web console: %v", err)
		}
	}

	if err := supervisor.Start(); err != nil {
		if apiServer != nil {
			log.Printf("Proxy failed to start: %v", err)
		} else {
			log.Fatalf("Proxy failed to start: %v", err)
		}
	}

	apiErr := make(chan error, 1)
	if apiServer != nil {
		go func() {
			apiErr <- apiServer.Wait()
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("Received %s, shutting down", sig)
	case err := <-apiErr:
		if err != nil {
			log.Printf("Web console stopped unexpectedly: %v", err)
		} else {
			log.Printf("Web console stopped")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	supervisor.Shutdown(ctx)
	cancel()
	// Close the broker first so SSE handlers return instead of keeping
	// http.Server.Shutdown waiting for long-lived streams.
	broker.Close()
	if apiServer != nil {
		apiCtx, apiCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = apiServer.Shutdown(apiCtx)
		apiCancel()
	}
}

func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if value := os.Getenv("NYXPROXY_CONFIG"); value != "" {
		return value
	}
	return "config.yaml"
}

func healthcheckURL(cfg *config.ProxyConfig) string {
	host := cfg.Web.ListenAddress
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	scheme := "http"
	if cfg.Web.TLSCertFile != "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/healthz", scheme, net.JoinHostPort(host, strconv.Itoa(cfg.Web.Port)))
}

func runHealthcheck(path string) int {
	cfg, err := config.LoadConfigFrom(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	if !cfg.Web.Enabled {
		return 0
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(healthcheckURL(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
```

Remove the unused `strings` import and `var _ = strings.TrimSpace`.

- [ ] **Step 4: Run tests and build**

```bash
go test ./cmd/proxy/... -count=1
go build ./cmd/proxy
go vet ./cmd/proxy/...
```

Expected: PASS and a successful build. The build embeds `internal/webui/dist`; the `.gitkeep` file keeps the directory present, and the placeholder page is used until the frontend is built.

- [ ] **Step 5: Commit checkpoint**

```bash
git add cmd/proxy
git commit -m "feat(main): wire supervisor, web console, config path, and healthcheck"
```

Only run if the user has authorized commits.

## Task 10: Frontend Scaffold, Types, i18n, Theme, Auth, and API Client

**Files:**
- Create: `web/package.json`
- Create: `web/next.config.ts`
- Create: `web/postcss.config.mjs`
- Create: `web/tsconfig.json`
- Create: `web/next-env.d.ts`
- Create: `web/.eslintrc.json`
- Create: `web/vitest.config.ts`
- Create: `web/vitest.setup.ts`
- Create: `web/app/globals.css`
- Create: `web/app/layout.tsx`
- Create: `web/app/providers.tsx`
- Create: `web/lib/types.ts`
- Create: `web/lib/api.ts`
- Create: `web/lib/cn.ts`
- Create: `web/lib/format.ts`
- Create: `web/lib/config-diff.ts`
- Create: `web/lib/validate.ts`
- Create: `web/lib/log-store.ts`
- Create: `web/lib/i18n/zh.ts`
- Create: `web/lib/i18n/en.ts`
- Create: `web/lib/i18n/index.tsx`
- Create: `web/lib/theme.tsx`
- Create: `web/lib/auth.tsx`
- Create: `web/lib/api.test.ts`
- Create: `web/lib/config-diff.test.ts`
- Create: `web/lib/validate.test.ts`
- Create: `web/lib/log-store.test.ts`
- Create: `web/lib/i18n.test.ts`
- Create: `web/pnpm-lock.yaml` (generated by `pnpm install`)

**Interfaces:**
- Produces:
  - `Config`, `Meta`, `ConfigResponse`, `Status`, `ApplyResult`, `InterfaceInfo`, `LogLine` in `lib/types.ts`
  - `api<T>`, `ApiError`, `setCSRFToken`
  - `useI18n()`, `I18nProvider`, `MessageKey`, `errorMessage`
  - `useTheme()`, `ThemeProvider`
  - `useAuth()`, `AuthProvider`
  - `restartSections(a,b)`
  - `validateConfig(cfg, passwordsSet)`
  - `appendLog(state,line,max)`, `recordGap(state,dropped)`, `emptyLogState()`
  - `formatBytes`, `formatDuration`, `formatDate`

- [ ] **Step 1: Create the package manifest and configuration**

Create `web/package.json`:

```json
{
  "name": "nyxproxy-web",
  "version": "0.1.0",
  "private": true,
  "packageManager": "pnpm@12.9.1",
  "scripts": {
    "dev": "next dev",
    "build": "cross-env NEXT_EXPORT=1 next build",
    "build:server": "next build",
    "start": "next start",
    "lint": "next lint",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "clsx": "2.1.1",
    "lucide-react": "0.544.0",
    "next": "15.5.4",
    "react": "19.1.1",
    "react-dom": "19.1.1"
  },
  "devDependencies": {
    "@tailwindcss/postcss": "4.1.13",
    "@testing-library/jest-dom": "6.8.0",
    "@testing-library/react": "16.3.0",
    "@testing-library/user-event": "14.6.1",
    "@types/node": "24.3.0",
    "@types/react": "19.1.12",
    "@types/react-dom": "19.1.9",
    "@vitejs/plugin-react": "5.0.4",
    "cross-env": "10.1.0",
    "eslint": "8.57.1",
    "eslint-config-next": "15.5.4",
    "jsdom": "27.0.0",
    "tailwindcss": "4.1.13",
    "typescript": "5.9.2",
    "vitest": "3.2.4"
  },
  "pnpm": {
    "onlyBuiltDependencies": [
      "@tailwindcss/oxide",
      "esbuild",
      "sharp"
    ]
  }
}
```

Create `web/next.config.ts`:

```ts
import type { NextConfig } from 'next';

const isExport = process.env.NEXT_EXPORT === '1';

const nextConfig: NextConfig = {
  output: isExport ? 'export' : undefined,
  trailingSlash: isExport,
  images: { unoptimized: true },
  async rewrites() {
    if (isExport) {
      return [];
    }
    const api = process.env.NYXPROXY_API ?? 'http://127.0.0.1:18081';
    return [{ source: '/api/:path*', destination: `${api}/api/:path*` }];
  },
};

export default nextConfig;
```

Create `web/postcss.config.mjs`:

```js
export default {
  plugins: {
    '@tailwindcss/postcss': {},
  },
};
```

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": false,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

Create `web/next-env.d.ts`:

```ts
/// <reference types="next" />
/// <reference types="next/image-types/global" />
```

Create `web/.eslintrc.json`:

```json
{
  "extends": "next/core-web-vitals"
}
```

Create `web/vitest.config.ts`:

```ts
import path from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
    },
  },
});
```

Create `web/vitest.setup.ts`:

```ts
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(() => cleanup());
```

- [ ] **Step 2: Install dependencies and commit the lockfile**

```bash
cd web
corepack enable
pnpm install
cd ..
```

Expected: `web/pnpm-lock.yaml` is created. Commit it with the frontend. Do not run `npm install`.

- [ ] **Step 3: Write failing tests for the pure libraries**

Create `web/lib/i18n.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { en } from './i18n/en';
import { zh } from './i18n/zh';

describe('i18n dictionaries', () => {
  it('have exactly the same keys', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort());
  });

  it('have no empty messages', () => {
    for (const [key, value] of Object.entries(zh)) {
      expect(value, key).not.toBe('');
    }
    for (const [key, value] of Object.entries(en)) {
      expect(value, key).not.toBe('');
    }
  });
});
```

Create `web/lib/config-diff.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import type { Config } from './types';
import { restartSections } from './config-diff';

function base(): Config {
  return {
    proxy: { type: 'socks5', listen_address: '0.0.0.0', listen_port: 1080, username: 'admin', password: '' },
    network: {
      interface_name: 'eth0',
      ipv4_enabled: false,
      ipv6_enabled: true,
      ipv6_subnet: '2001:db8::/64',
      dns_source_address: '',
      rotate_ipv6: true,
      ipv6_pool_size: 64,
      ipv6_max_usage: 100,
      ipv6_max_age: 30,
    },
    monitoring: { enabled: false, port: 9090, allow_remote: false },
    logging: { debug_level: 1 },
    web: {
      enabled: true,
      listen_address: '127.0.0.1',
      port: 18081,
      username: 'admin',
      password: '',
      tls_cert_file: '',
      tls_key_file: '',
      trust_proxy_headers: false,
      cookie_secure: false,
    },
  };
}

describe('restartSections', () => {
  it('detects proxy, network, and logging changes', () => {
    const before = base();
    const after = structuredClone(before);
    after.proxy.listen_port = 1081;
    after.network.ipv6_pool_size = 128;
    after.logging.debug_level = 2;
    expect(restartSections(before, after)).toEqual(['proxy', 'network', 'logging']);
  });

  it('ignores monitoring, web credentials, and web settings', () => {
    const before = base();
    const after = structuredClone(before);
    after.monitoring.port = 9091;
    after.web.username = 'other';
    after.web.port = 18082;
    expect(restartSections(before, after)).toEqual([]);
  });
});
```

Create `web/lib/validate.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import type { Config } from './types';
import { validateConfig } from './validate';

function valid(): Config {
  return {
    proxy: { type: 'socks5', listen_address: '0.0.0.0', listen_port: 1080, username: 'admin', password: '' },
    network: {
      interface_name: 'eth0',
      ipv4_enabled: false,
      ipv6_enabled: true,
      ipv6_subnet: '2001:db8::/64',
      dns_source_address: '',
      rotate_ipv6: true,
      ipv6_pool_size: 64,
      ipv6_max_usage: 100,
      ipv6_max_age: 30,
    },
    monitoring: { enabled: false, port: 9090, allow_remote: false },
    logging: { debug_level: 1 },
    web: {
      enabled: true,
      listen_address: '127.0.0.1',
      port: 18081,
      username: 'admin',
      password: '',
      tls_cert_file: '',
      tls_key_file: '',
      trust_proxy_headers: false,
      cookie_secure: false,
    },
  };
}

const passwordsSet = { 'proxy.password': true, 'web.password': true };

describe('validateConfig', () => {
  it('accepts empty passwords when a password is already stored', () => {
    expect(validateConfig(valid(), passwordsSet)).toEqual({});
  });

  it('requires credentials when none are stored', () => {
    const errors = validateConfig(valid(), {});
    expect(errors['proxy.password']).toBeTruthy();
    expect(errors['web.password']).toBeTruthy();
  });

  it('checks ports and subnet format', () => {
    const cfg = valid();
    cfg.proxy.listen_port = 0;
    cfg.monitoring.enabled = true;
    cfg.monitoring.port = 70000;
    cfg.network.ipv6_subnet = 'not-a-prefix';
    const errors = validateConfig(cfg, passwordsSet);
    expect(errors['proxy.listen_port']).toBeTruthy();
    expect(errors['monitoring.port']).toBeTruthy();
    expect(errors['network.ipv6_subnet']).toBeTruthy();
  });

  it('rejects overlapping listeners', () => {
    const cfg = valid();
    cfg.monitoring.enabled = true;
    cfg.monitoring.port = 1080;
    cfg.web.port = 1080;
    const errors = validateConfig(cfg, passwordsSet);
    expect(errors['monitoring.port']).toBeTruthy();
    expect(errors['web.port']).toBeTruthy();
  });

  it('requires at least one enabled protocol', () => {
    const cfg = valid();
    cfg.network.ipv4_enabled = false;
    cfg.network.ipv6_enabled = false;
    expect(validateConfig(cfg, passwordsSet)['network.ipv4_enabled']).toBeTruthy();
  });
});
```

Create `web/lib/log-store.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { appendLog, emptyLogState, recordGap } from './log-store';

describe('log store', () => {
  it('keeps at most 2000 lines', () => {
    let state = emptyLogState();
    for (let i = 0; i < 2005; i++) {
      state = appendLog(state, { seq: i + 1, ts: '2026-10-18T00:00:00Z', line: `line-${i}` });
    }
    expect(state.lines).toHaveLength(2000);
    expect(state.lines[0].line).toBe('line-5');
  });

  it('accumulates dropped counts from gap events', () => {
    let state = emptyLogState();
    state = recordGap(state, 3);
    state = recordGap(state, 4);
    expect(state.dropped).toBe(7);
  });

  it('starts with empty lines and no dropped records', () => {
    expect(emptyLogState()).toEqual({ lines: [], dropped: 0 });
  });
});
```

Create `web/lib/api.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api, setCSRFToken } from './api';

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken('');
});

describe('api client', () => {
  it('adds the CSRF header for mutations', async () => {
    const fetchMock = mockFetch(200, { ok: true });
    vi.stubGlobal('fetch', fetchMock);
    setCSRFToken('token-123');

    await api('/api/config', { method: 'PUT', body: '{}' });

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect((init.headers as Headers).get('X-CSRF-Token')).toBe('token-123');
    expect((init.headers as Headers).get('Content-Type')).toBe('application/json');
  });

  it('maps the error envelope to ApiError', async () => {
    vi.stubGlobal('fetch', mockFetch(422, {
      error: { code: 'validation_failed', message: 'invalid', fields: { 'proxy.listen_port': 'bad' } },
    }));

    await expect(api('/api/config', { method: 'PUT', body: '{}' })).rejects.toMatchObject({
      code: 'validation_failed',
      fields: { 'proxy.listen_port': 'bad' },
      status: 422,
    });
  });

  it('throws ApiError instances', async () => {
    vi.stubGlobal('fetch', mockFetch(500, { error: { code: 'internal_error', message: 'boom' } }));
    await expect(api('/api/status')).rejects.toBeInstanceOf(ApiError);
  });
});
```

- [ ] **Step 4: Run the tests and verify they fail**

```bash
cd web
pnpm test
cd ..
```

Expected: failures because the library modules do not exist yet.

- [ ] **Step 5: Implement the shared types and API client**

Create `web/lib/types.ts`:

```ts
export type ProxyType = 'socks5' | 'https';

export interface ProxySettings {
  type: ProxyType;
  listen_address: string;
  listen_port: number;
  username: string;
  password: string;
}

export interface NetworkSettings {
  interface_name: string;
  ipv4_enabled: boolean;
  ipv6_enabled: boolean;
  ipv6_subnet: string;
  dns_source_address: string;
  rotate_ipv6: boolean;
  ipv6_pool_size: number;
  ipv6_max_usage: number;
  ipv6_max_age: number;
}

export interface MonitoringSettings {
  enabled: boolean;
  port: number;
  allow_remote: boolean;
}

export interface LoggingSettings {
  debug_level: number;
}

export interface WebSettings {
  enabled: boolean;
  listen_address: string;
  port: number;
  username: string;
  password: string;
  tls_cert_file: string;
  tls_key_file: string;
  trust_proxy_headers: boolean;
  cookie_secure: boolean;
}

export interface Config {
  proxy: ProxySettings;
  network: NetworkSettings;
  monitoring: MonitoringSettings;
  logging: LoggingSettings;
  web: WebSettings;
}

export interface Meta {
  path: string;
  generation: number;
  locked: Record<string, 'env' | 'restart_required'>;
  passwords_set: Record<string, boolean>;
}

export interface ConfigResponse {
  config: Config;
  meta: Meta;
}

export interface ApplyResult {
  changed: boolean;
  generation: number;
  restarted: { proxy: boolean; monitoring: boolean };
  reauth_required: boolean;
}

export interface SessionResponse {
  username: string;
  csrf_token: string;
  expires_at: string;
}

export interface ProxyStatus {
  state: 'starting' | 'running' | 'error' | 'stopped';
  type: string;
  listen: string;
  protocol: number;
  started_at?: string;
  error: string;
}

export interface Status {
  version: string;
  uptime_seconds: number;
  proxy: ProxyStatus;
  monitoring: { enabled: boolean; state: 'stopped' | 'running' | 'error'; error: string };
  pool: {
    enabled: boolean;
    size: number;
    prefix: string;
    rotations: number;
    max_usage: number;
    max_age_minutes: number;
  };
  metrics: {
    active_connections: number;
    total_connections: number;
    total_requests: number;
    bytes_sent: number;
    bytes_received: number;
  };
  config: {
    path: string;
    generation: number;
    last_applied_at?: string;
    last_apply: { ok: boolean; error: string; at: string; generation: number };
  };
  platform: { os: string; ip_command: boolean; warnings: string[] };
  apply: { in_progress: boolean; step: string };
}

export interface InterfaceInfo {
  name: string;
  addresses: string[];
  suggested_prefix: string;
}

export interface LogLine {
  seq: number;
  ts: string;
  line: string;
}
```

Create `web/lib/api.ts`:

```ts
let csrfToken = '';

export function setCSRFToken(token: string): void {
  csrfToken = token;
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    fields?: Record<string, string>;
    rolled_back?: boolean;
  };
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields?: Record<string, string>;
  readonly rolledBack?: boolean;

  constructor(message: string, status: number, code: string, fields?: Record<string, string>, rolledBack?: boolean) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.rolledBack = rolledBack;
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken);
  }

  const response = await fetch(path, { ...init, headers, credentials: 'same-origin' });
  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    const envelope = (payload ?? {}) as ErrorEnvelope;
    const error = new ApiError(
      envelope.error?.message ?? `HTTP ${response.status}`,
      response.status,
      envelope.error?.code ?? 'internal_error',
      envelope.error?.fields,
      envelope.error?.rolled_back,
    );
    if (response.status === 401 && path !== '/api/auth/login') {
      window.dispatchEvent(new Event('nyxproxy:unauthenticated'));
    }
    throw error;
  }

  return payload as T;
}
```

Create `web/lib/cn.ts`:

```ts
import clsx, { type ClassValue } from 'clsx';

export function cn(...inputs: ClassValue[]): string {
  return clsx(inputs);
}
```

Create `web/lib/config-diff.ts`:

```ts
import type { Config } from './types';

export type RestartSection = 'proxy' | 'network' | 'logging';

export function restartSections(before: Config, after: Config): RestartSection[] {
  const changed: RestartSection[] = [];
  if (!same(before.proxy, after.proxy)) changed.push('proxy');
  if (!same(before.network, after.network)) changed.push('network');
  if (!same(before.logging, after.logging)) changed.push('logging');
  return changed;
}

export function hasChanges(before: Config, after: Config): boolean {
  return JSON.stringify(before) !== JSON.stringify(after);
}

function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
```

Create `web/lib/validate.ts`:

```ts
import type { Config } from './types';

function isIPv6Prefix(value: string): boolean {
  const [addr, bits, extra] = value.split('/');
  if (extra !== undefined || bits !== '64' || !addr || !addr.includes(':')) return false;
  if (!/^[0-9a-fA-F:]+$/.test(addr)) return false;
  if ((addr.match(/::/g) ?? []).length > 1) return false;
  const groups = addr.split(':').filter(Boolean);
  return groups.length <= 8 && groups.every((group) => group.length <= 4);
}

function portError(port: number): string | undefined {
  if (!Number.isInteger(port) || port < 1 || port > 65535) return 'must be between 1 and 65535';
  return undefined;
}

function listenersOverlap(aAddr: string, aPort: number, bAddr: string, bPort: number): boolean {
  if (aPort !== bPort) return false;
  const a = aAddr || '0.0.0.0';
  const b = bAddr || '0.0.0.0';
  if (a === b) return true;
  return a === '0.0.0.0' || a === '::' || b === '0.0.0.0' || b === '::';
}

export function validateConfig(
  cfg: Config,
  passwordsSet: Record<string, boolean>,
): Record<string, string> {
  const errors: Record<string, string> = {};
  const set = (field: string, message: string) => {
    if (!errors[field]) errors[field] = message;
  };

  if (cfg.proxy.type !== 'socks5' && cfg.proxy.type !== 'https') {
    set('proxy.type', "must be 'https' or 'socks5'");
  }
  if (!cfg.proxy.username) set('proxy.username', 'must be set');
  if (!cfg.proxy.password && !passwordsSet['proxy.password']) {
    set('proxy.password', 'must be set');
  }
  const proxyPortError = portError(cfg.proxy.listen_port);
  if (proxyPortError) set('proxy.listen_port', proxyPortError);

  if (!cfg.network.ipv4_enabled && !cfg.network.ipv6_enabled) {
    set('network.ipv4_enabled', 'at least one of ipv4_enabled or ipv6_enabled must be true');
  }
  if (cfg.network.ipv6_subnet && !isIPv6Prefix(cfg.network.ipv6_subnet)) {
    set('network.ipv6_subnet', 'must be a valid IPv6 /64');
  }
  if (cfg.network.rotate_ipv6) {
    if (cfg.network.ipv6_pool_size <= 0) set('network.ipv6_pool_size', 'must be greater than 0');
    if (cfg.network.ipv6_max_usage <= 0) set('network.ipv6_max_usage', 'must be greater than 0');
    if (cfg.network.ipv6_max_age <= 0) set('network.ipv6_max_age', 'must be greater than 0');
  }

  let monitorPortOK = true;
  if (cfg.monitoring.enabled) {
    const err = portError(cfg.monitoring.port);
    if (err) {
      set('monitoring.port', err);
      monitorPortOK = false;
    }
  }

  if (cfg.web.enabled) {
    if (!cfg.web.username) set('web.username', 'must be set');
    if (!cfg.web.password && !passwordsSet['web.password']) set('web.password', 'must be set');
    const err = portError(cfg.web.port);
    if (err) set('web.port', err);
  } else if (portError(cfg.web.port)) {
    // Keep the dormant value valid so enabling the console later cannot save an
    // out-of-range port.
    set('web.port', portError(cfg.web.port)!);
  }

  const webPortOK = !errors['web.port'];
  const monitorAddr = cfg.monitoring.allow_remote ? '0.0.0.0' : '127.0.0.1';
  if (proxyPortError === undefined && monitorPortOK && cfg.monitoring.enabled &&
      listenersOverlap(cfg.proxy.listen_address, cfg.proxy.listen_port, monitorAddr, cfg.monitoring.port)) {
    set('monitoring.port', 'conflicts with proxy.listen_port');
  }
  if (proxyPortError === undefined && webPortOK && cfg.web.enabled &&
      listenersOverlap(cfg.proxy.listen_address, cfg.proxy.listen_port, cfg.web.listen_address, cfg.web.port)) {
    set('web.port', 'conflicts with proxy.listen_port');
  }
  if (monitorPortOK && webPortOK && cfg.monitoring.enabled && cfg.web.enabled &&
      listenersOverlap(monitorAddr, cfg.monitoring.port, cfg.web.listen_address, cfg.web.port)) {
    set('web.port', 'conflicts with monitoring.port');
  }

  return errors;
}
```

Create `web/lib/log-store.ts`:

```ts
import type { LogLine } from './types';

export interface LogState {
  lines: LogLine[];
  dropped: number;
}

export const MAX_LINES = 2000;

export function emptyLogState(): LogState {
  return { lines: [], dropped: 0 };
}

export function appendLog(state: LogState, line: LogLine, max = MAX_LINES): LogState {
  const lines = [...state.lines, line];
  return {
    lines: lines.length > max ? lines.slice(lines.length - max) : lines,
    dropped: state.dropped,
  };
}

export function recordGap(state: LogState, dropped: number): LogState {
  return { ...state, dropped: state.dropped + dropped };
}
```

Create `web/lib/format.ts`:

```ts
export function formatBytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const parts: string[] = [];
  if (days) parts.push(`${days}d`);
  if (hours) parts.push(`${hours}h`);
  if (minutes) parts.push(`${minutes}m`);
  parts.push(`${secs}s`);
  return parts.join(' ');
}

export function formatDate(value: string | undefined, lang: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(date);
}
```

- [ ] **Step 6: Implement the i18n dictionaries and provider**

Create `web/lib/i18n/zh.ts`:

```ts
export const zh = {
  'app.name': 'NyxProxy',
  'nav.dashboard': '仪表盘',
  'nav.config': '配置',
  'nav.logs': '日志',
  'nav.logout': '退出登录',
  'top.connected': '已连接',
  'top.disconnected': '离线',
  'top.theme': '切换主题',
  'top.language': '切换语言',
  'theme.light': '浅色',
  'theme.dark': '深色',
  'theme.system': '跟随系统',
  'login.title': '登录管理面板',
  'login.subtitle': '使用配置中的 Web 用户名和密码登录。',
  'login.username': '用户名',
  'login.password': '密码',
  'login.submit': '登录',
  'login.error': '登录失败',
  'common.loading': '加载中…',
  'common.save': '保存并应用',
  'common.saving': '正在保存…',
  'common.cancel': '取消',
  'common.retry': '重试启动',
  'common.reload': '重新加载',
  'common.close': '关闭',
  'common.enabled': '启用',
  'common.disabled': '禁用',
  'common.unknown': '未知',
  'dashboard.title': '仪表盘',
  'dashboard.proxy': '代理',
  'dashboard.pool': 'IPv6 地址池',
  'dashboard.traffic': '流量',
  'dashboard.config': '配置',
  'dashboard.warnings': '警告',
  'dashboard.retry': '重试启动',
  'dashboard.state.running': '运行中',
  'dashboard.state.starting': '启动中',
  'dashboard.state.error': '错误',
  'dashboard.state.stopped': '已停止',
  'dashboard.type': '类型',
  'dashboard.listen': '监听地址',
  'dashboard.protocol': '协议',
  'dashboard.uptime': '运行时长',
  'dashboard.pool_size': '池大小',
  'dashboard.prefix': '当前前缀',
  'dashboard.rotations': '已轮换',
  'dashboard.max_usage': '最大使用次数',
  'dashboard.max_age': '最大存活时间',
  'dashboard.active_connections': '活跃连接',
  'dashboard.total_connections': '累计连接',
  'dashboard.requests': '请求数',
  'dashboard.bytes_sent': '发送字节',
  'dashboard.bytes_received': '接收字节',
  'dashboard.generation': '配置版本',
  'dashboard.last_applied': '上次应用',
  'dashboard.config_path': '配置文件',
  'dashboard.monitoring': '监控服务',
  'dashboard.ip_missing': '未找到 ip 命令，IPv6 轮换不可用。',
  'dashboard.last_apply_failed': '上次保存失败',
  'dashboard.proxy_not_running': '代理未运行',
  'dashboard.apply_in_progress': '正在应用配置：{step}',
  'config.title': '配置',
  'config.proxy': '代理',
  'config.network': '网络',
  'config.rotation': '轮换',
  'config.monitoring': '监控',
  'config.logging': '日志',
  'config.web': 'Web 面板',
  'config.advanced': '高级',
  'config.unsaved': '有未保存的修改',
  'config.unsaved_confirm': '有未保存的修改，确定离开吗？',
  'config.applying': '正在应用…',
  'config.last_ok': '上次应用成功（版本 {generation}）',
  'config.last_failed': '上次应用失败：{error}',
  'config.locked_env': '由环境变量管理',
  'config.locked_restart': '重启生效',
  'config.password_set': '已设置',
  'config.password_keep': '已设置，留空保持不变',
  'config.interface': '网卡',
  'config.save': '保存并应用',
  'config.confirm_title': '确认应用配置',
  'config.confirm_body': '修改代理、网络或日志设置会重启代理并中断现有连接。继续吗？',
  'config.saved': '配置已保存并应用',
  'config.invalid': '请修正标红的字段后再保存',
  'config.failed': '配置应用失败',
  'config.step.validating': '校验中',
  'config.step.stopping': '停止旧代理',
  'config.step.starting_proxy': '启动新代理',
  'config.step.starting_monitoring': '启动监控服务',
  'config.step.committing': '写入配置文件',
  'config.field.proxy.type': '代理类型',
  'config.field.proxy.listen_address': '监听地址',
  'config.field.proxy.listen_port': '监听端口',
  'config.field.proxy.username': '代理用户名',
  'config.field.proxy.password': '代理密码',
  'config.field.network.interface_name': '网络接口',
  'config.field.network.ipv4_enabled': '启用 IPv4',
  'config.field.network.ipv6_enabled': '启用 IPv6',
  'config.field.network.ipv6_subnet': 'IPv6 /64 子网',
  'config.field.network.dns_source_address': 'DNS 源地址',
  'config.field.network.rotate_ipv6': '启用 IPv6 轮换',
  'config.field.network.ipv6_pool_size': '地址池大小',
  'config.field.network.ipv6_max_usage': '单地址最大使用次数',
  'config.field.network.ipv6_max_age': '单地址最大存活时间（分钟）',
  'config.field.monitoring.enabled': '启用监控端点',
  'config.field.monitoring.port': '监控端口',
  'config.field.monitoring.allow_remote': '允许远程访问监控',
  'config.field.logging.debug_level': '调试级别',
  'config.field.web.enabled': '启用 Web 面板',
  'config.field.web.listen_address': '面板监听地址',
  'config.field.web.port': '面板端口',
  'config.field.web.username': '面板用户名',
  'config.field.web.password': '面板密码',
  'config.field.web.tls_cert_file': 'TLS 证书文件',
  'config.field.web.tls_key_file': 'TLS 私钥文件',
  'config.field.web.trust_proxy_headers': '信任反向代理头',
  'config.field.web.cookie_secure': 'Cookie Secure',
  'logs.title': '实时日志',
  'logs.pause': '暂停滚动',
  'logs.resume': '继续滚动',
  'logs.clear': '清空',
  'logs.filter': '过滤日志',
  'logs.connected': '已连接',
  'logs.disconnected': '已断开',
  'logs.new': '有 {count} 条新日志',
  'logs.empty': '暂无日志',
  'logs.gap': '有 {count} 条日志因缓冲区覆盖而丢失',
  'errors.invalid_credentials': '用户名或密码错误',
  'errors.unauthenticated': '登录已过期，请重新登录',
  'errors.csrf_failed': '安全校验失败，请刷新页面后重试',
  'errors.origin_mismatch': '请求来源校验失败',
  'errors.rate_limited': '登录尝试过多，请稍后再试',
  'errors.validation_failed': '配置校验失败',
  'errors.field_locked': '字段被锁定，无法修改',
  'errors.restart_required': '该字段修改后需要重启进程',
  'errors.apply_in_progress': '已有配置正在应用，请稍候',
  'errors.apply_failed': '配置应用失败',
  'errors.persist_failed': '写入配置文件失败',
  'errors.proxy_already_running': '代理已经在运行',
  'errors.payload_too_large': '请求体过大',
  'errors.unsupported_media_type': '请求格式不受支持',
  'errors.method_not_allowed': '请求方法不允许',
  'errors.not_found': '接口不存在',
  'errors.internal_error': '服务器内部错误',
  'errors.invalid_json': '请求 JSON 格式错误',
  'errors.network': '网络请求失败',
} as const;

export type MessageKey = keyof typeof zh;
```

Create `web/lib/i18n/en.ts`:

```ts
import type { MessageKey } from './zh';

export const en: Record<MessageKey, string> = {
  'app.name': 'NyxProxy',
  'nav.dashboard': 'Dashboard',
  'nav.config': 'Configuration',
  'nav.logs': 'Logs',
  'nav.logout': 'Sign out',
  'top.connected': 'Connected',
  'top.disconnected': 'Offline',
  'top.theme': 'Toggle theme',
  'top.language': 'Toggle language',
  'theme.light': 'Light',
  'theme.dark': 'Dark',
  'theme.system': 'System',
  'login.title': 'Sign in to the console',
  'login.subtitle': 'Use the web username and password from your configuration.',
  'login.username': 'Username',
  'login.password': 'Password',
  'login.submit': 'Sign in',
  'login.error': 'Sign-in failed',
  'common.loading': 'Loading…',
  'common.save': 'Save and apply',
  'common.saving': 'Saving…',
  'common.cancel': 'Cancel',
  'common.retry': 'Retry start',
  'common.reload': 'Reload',
  'common.close': 'Close',
  'common.enabled': 'Enabled',
  'common.disabled': 'Disabled',
  'common.unknown': 'Unknown',
  'dashboard.title': 'Dashboard',
  'dashboard.proxy': 'Proxy',
  'dashboard.pool': 'IPv6 pool',
  'dashboard.traffic': 'Traffic',
  'dashboard.config': 'Configuration',
  'dashboard.warnings': 'Warnings',
  'dashboard.retry': 'Retry start',
  'dashboard.state.running': 'Running',
  'dashboard.state.starting': 'Starting',
  'dashboard.state.error': 'Error',
  'dashboard.state.stopped': 'Stopped',
  'dashboard.type': 'Type',
  'dashboard.listen': 'Listen',
  'dashboard.protocol': 'Protocol',
  'dashboard.uptime': 'Uptime',
  'dashboard.pool_size': 'Pool size',
  'dashboard.prefix': 'Prefix',
  'dashboard.rotations': 'Rotated',
  'dashboard.max_usage': 'Max uses',
  'dashboard.max_age': 'Max age',
  'dashboard.active_connections': 'Active connections',
  'dashboard.total_connections': 'Total connections',
  'dashboard.requests': 'Requests',
  'dashboard.bytes_sent': 'Bytes sent',
  'dashboard.bytes_received': 'Bytes received',
  'dashboard.generation': 'Generation',
  'dashboard.last_applied': 'Last applied',
  'dashboard.config_path': 'Config file',
  'dashboard.monitoring': 'Monitoring',
  'dashboard.ip_missing': 'The ip command was not found; IPv6 rotation is unavailable.',
  'dashboard.last_apply_failed': 'Last apply failed',
  'dashboard.proxy_not_running': 'Proxy is not running',
  'dashboard.apply_in_progress': 'Applying configuration: {step}',
  'config.title': 'Configuration',
  'config.proxy': 'Proxy',
  'config.network': 'Network',
  'config.rotation': 'Rotation',
  'config.monitoring': 'Monitoring',
  'config.logging': 'Logging',
  'config.web': 'Web console',
  'config.advanced': 'Advanced',
  'config.unsaved': 'Unsaved changes',
  'config.unsaved_confirm': 'You have unsaved changes. Leave anyway?',
  'config.applying': 'Applying…',
  'config.last_ok': 'Last apply succeeded (generation {generation})',
  'config.last_failed': 'Last apply failed: {error}',
  'config.locked_env': 'Managed by environment',
  'config.locked_restart': 'Restart required',
  'config.password_set': 'Set',
  'config.password_keep': 'Set; leave blank to keep',
  'config.interface': 'Interface',
  'config.save': 'Save and apply',
  'config.confirm_title': 'Apply configuration',
  'config.confirm_body': 'Changing proxy, network, or logging settings restarts the proxy and interrupts active connections. Continue?',
  'config.saved': 'Configuration saved and applied',
  'config.invalid': 'Fix the highlighted fields before saving',
  'config.failed': 'Configuration apply failed',
  'config.step.validating': 'Validating',
  'config.step.stopping': 'Stopping old proxy',
  'config.step.starting_proxy': 'Starting new proxy',
  'config.step.starting_monitoring': 'Starting monitoring',
  'config.step.committing': 'Writing config file',
  'config.field.proxy.type': 'Proxy type',
  'config.field.proxy.listen_address': 'Listen address',
  'config.field.proxy.listen_port': 'Listen port',
  'config.field.proxy.username': 'Proxy username',
  'config.field.proxy.password': 'Proxy password',
  'config.field.network.interface_name': 'Interface',
  'config.field.network.ipv4_enabled': 'Enable IPv4',
  'config.field.network.ipv6_enabled': 'Enable IPv6',
  'config.field.network.ipv6_subnet': 'IPv6 /64 subnet',
  'config.field.network.dns_source_address': 'DNS source address',
  'config.field.network.rotate_ipv6': 'Enable IPv6 rotation',
  'config.field.network.ipv6_pool_size': 'Pool size',
  'config.field.network.ipv6_max_usage': 'Max uses per address',
  'config.field.network.ipv6_max_age': 'Max age per address (minutes)',
  'config.field.monitoring.enabled': 'Enable monitoring endpoint',
  'config.field.monitoring.port': 'Monitoring port',
  'config.field.monitoring.allow_remote': 'Allow remote monitoring',
  'config.field.logging.debug_level': 'Debug level',
  'config.field.web.enabled': 'Enable web console',
  'config.field.web.listen_address': 'Console listen address',
  'config.field.web.port': 'Console port',
  'config.field.web.username': 'Console username',
  'config.field.web.password': 'Console password',
  'config.field.web.tls_cert_file': 'TLS certificate file',
  'config.field.web.tls_key_file': 'TLS key file',
  'config.field.web.trust_proxy_headers': 'Trust reverse proxy headers',
  'config.field.web.cookie_secure': 'Cookie Secure',
  'logs.title': 'Live logs',
  'logs.pause': 'Pause scrolling',
  'logs.resume': 'Resume scrolling',
  'logs.clear': 'Clear',
  'logs.filter': 'Filter logs',
  'logs.connected': 'Connected',
  'logs.disconnected': 'Disconnected',
  'logs.new': '{count} new log lines',
  'logs.empty': 'No log lines yet',
  'logs.gap': '{count} log lines were dropped by the ring buffer',
  'errors.invalid_credentials': 'Invalid username or password',
  'errors.unauthenticated': 'Your session expired; sign in again',
  'errors.csrf_failed': 'Security validation failed; refresh the page and retry',
  'errors.origin_mismatch': 'Request origin validation failed',
  'errors.rate_limited': 'Too many login attempts; try again later',
  'errors.validation_failed': 'Configuration validation failed',
  'errors.field_locked': 'This field is locked',
  'errors.restart_required': 'Changing this field requires a process restart',
  'errors.apply_in_progress': 'Another apply is in progress',
  'errors.apply_failed': 'Configuration apply failed',
  'errors.persist_failed': 'Failed to write the configuration file',
  'errors.proxy_already_running': 'The proxy is already running',
  'errors.payload_too_large': 'Request body is too large',
  'errors.unsupported_media_type': 'Unsupported request format',
  'errors.method_not_allowed': 'Method not allowed',
  'errors.not_found': 'Endpoint not found',
  'errors.internal_error': 'Internal server error',
  'errors.invalid_json': 'Invalid request JSON',
  'errors.network': 'Network request failed',
};
```

Create `web/lib/i18n/index.tsx`:

```tsx
'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ApiError } from '@/lib/api';
import { en } from './en';
import { zh } from './zh';

export type Lang = 'zh' | 'en';
export type MessageKey = keyof typeof zh;

type Vars = Record<string, string | number>;
type Dictionary = Record<MessageKey, string>;

const dictionaries: Record<Lang, Dictionary> = { zh, en };

interface I18nValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: MessageKey, vars?: Vars) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

function detectLang(): Lang {
  const saved = localStorage.getItem('nyxproxy-lang');
  if (saved === 'zh' || saved === 'en') return saved;
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) => String(vars[key] ?? `{${key}}`));
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>('en');

  useEffect(() => {
    setLangState(detectLang());
  }, []);

  useEffect(() => {
    localStorage.setItem('nyxproxy-lang', lang);
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
  }, [lang]);

  const setLang = useCallback((next: Lang) => setLangState(next), []);
  const t = useCallback(
    (key: MessageKey, vars?: Vars) => interpolate(dictionaries[lang][key] ?? zh[key] ?? key, vars),
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error('useI18n must be used inside I18nProvider');
  return value;
}

export function errorMessage(t: I18nValue['t'], error: unknown): string {
  if (error instanceof ApiError) {
    const key = `errors.${error.code}` as MessageKey;
    if (key in zh) return t(key);
    return error.message;
  }
  if (error instanceof Error) return error.message;
  return t('errors.network');
}
```

- [ ] **Step 7: Implement theme and auth providers**

Create `web/lib/theme.tsx`:

```tsx
'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

export type Theme = 'light' | 'dark' | 'system';

interface ThemeValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  cycle: () => void;
}

const ThemeContext = createContext<ThemeValue | null>(null);

function apply(theme: Theme): void {
  const dark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', dark);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>('system');

  useEffect(() => {
    const saved = localStorage.getItem('nyxproxy-theme');
    if (saved === 'light' || saved === 'dark' || saved === 'system') {
      setThemeState(saved);
    }
  }, []);

  useEffect(() => {
    localStorage.setItem('nyxproxy-theme', theme);
    apply(theme);
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const listener = () => apply(theme);
    media.addEventListener('change', listener);
    return () => media.removeEventListener('change', listener);
  }, [theme]);

  const setTheme = useCallback((next: Theme) => setThemeState(next), []);
  const cycle = useCallback(() => {
    setThemeState((current) => (current === 'light' ? 'dark' : current === 'dark' ? 'system' : 'light'));
  }, []);
  const value = useMemo(() => ({ theme, setTheme, cycle }), [theme, setTheme, cycle]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  const value = useContext(ThemeContext);
  if (!value) throw new Error('useTheme must be used inside ThemeProvider');
  return value;
}
```

Create `web/lib/auth.tsx`:

```tsx
'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, setCSRFToken } from '@/lib/api';
import type { SessionResponse } from '@/lib/types';

interface AuthValue {
  username: string | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [username, setUsername] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const session = await api<SessionResponse>('/api/auth/session');
      setCSRFToken(session.csrf_token);
      setUsername(session.username);
    } catch {
      setCSRFToken('');
      setUsername(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const onUnauthenticated = () => {
      setCSRFToken('');
      setUsername(null);
    };
    window.addEventListener('nyxproxy:unauthenticated', onUnauthenticated);
    return () => window.removeEventListener('nyxproxy:unauthenticated', onUnauthenticated);
  }, [refresh]);

  const login = useCallback(async (user: string, password: string) => {
    const session = await api<SessionResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username: user, password }),
    });
    setCSRFToken(session.csrf_token);
    setUsername(session.username);
  }, []);

  const logout = useCallback(async () => {
    try {
      await api('/api/auth/logout', { method: 'POST' });
    } finally {
      setCSRFToken('');
      setUsername(null);
    }
  }, []);

  const value = useMemo(
    () => ({ username, loading, login, logout, refresh }),
    [username, loading, login, logout, refresh],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}
```

- [ ] **Step 8: Implement global styles, root layout, and providers**

Create `web/app/globals.css`:

```css
@import "tailwindcss";

@custom-variant dark (&:where(.dark, .dark *));

:root {
  color-scheme: light;
}

.dark {
  color-scheme: dark;
}

html,
body {
  min-height: 100%;
}

body {
  @apply bg-slate-50 text-slate-900 antialiased dark:bg-slate-950 dark:text-slate-100;
}
```

Create `web/app/layout.tsx`:

```tsx
import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './globals.css';
import { Providers } from './providers';

export const metadata: Metadata = {
  title: 'NyxProxy',
  description: 'NyxProxy web console',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

Create `web/app/providers.tsx`:

```tsx
'use client';

import type { ReactNode } from 'react';
import { AuthProvider } from '@/lib/auth';
import { I18nProvider } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme';

export function Providers({ children }: { children: ReactNode }) {
  return (
    <I18nProvider>
      <ThemeProvider>
        <AuthProvider>{children}</AuthProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
```

- [ ] **Step 9: Run the frontend tests and typecheck**

```bash
cd web
pnpm test
pnpm exec tsc --noEmit
cd ..
```

Expected: the four test files pass and `tsc` exits 0. If `tsc` reports missing Next types, run `pnpm build` once to generate `.next/types`, then re-run `pnpm exec tsc --noEmit`. `next-env.d.ts` is committed, so it should not be missing.

- [ ] **Step 10: Commit checkpoint**

```bash
git add web
git commit -m "feat(web): scaffold Next.js console with pnpm, i18n, auth, and API client"
```

Only run if the user has authorized commits. Commit `web/pnpm-lock.yaml` in this step.

## Task 11: UI Primitives, App Shell, Login, and Dirty-State Guard

**Files:**
- Create: `web/components/ui/Button.tsx`
- Create: `web/components/ui/Input.tsx`
- Create: `web/components/ui/Select.tsx`
- Create: `web/components/ui/Switch.tsx`
- Create: `web/components/ui/Card.tsx`
- Create: `web/components/ui/Badge.tsx`
- Create: `web/components/ui/Banner.tsx`
- Create: `web/components/ui/Dialog.tsx`
- Create: `web/components/ui/Skeleton.tsx`
- Create: `web/components/ui/Toast.tsx`
- Create: `web/components/AppShell.tsx`
- Create: `web/components/LoginForm.tsx`
- Create: `web/components/LoginForm.test.tsx`
- Create: `web/lib/dirty.tsx`
- Create: `web/app/(auth)/login/page.tsx`
- Create: `web/app/(app)/layout.tsx`
- Modify: `web/app/providers.tsx`

**Interfaces:**
- Consumes: `useI18n`, `useTheme`, `useAuth`, `cn`, `Button`
- Produces:
  - UI components: `Button`, `Input`, `Select`, `Switch`, `Card`, `Badge`, `Banner`, `Dialog`, `Skeleton`, `ToastProvider`, `useToast`
  - `AppShell({children})`
  - `LoginForm({onSubmit, error})`
  - `DirtyProvider`, `useDirty`

- [ ] **Step 1: Write the failing login form test**

Create `web/components/LoginForm.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { LoginForm } from './LoginForm';
import { I18nProvider } from '@/lib/i18n';

describe('LoginForm', () => {
  it('submits the entered credentials', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(
      <I18nProvider>
        <LoginForm onSubmit={onSubmit} />
      </I18nProvider>,
    );

    await userEvent.type(screen.getByLabelText(/username/i), 'admin');
    await userEvent.type(screen.getByLabelText(/password/i), 'secret');
    await userEvent.click(screen.getByRole('button', { name: /sign in|登录/i }));

    expect(onSubmit).toHaveBeenCalledWith('admin', 'secret');
  });

  it('renders a server error', () => {
    render(
      <I18nProvider>
        <LoginForm onSubmit={vi.fn()} error="invalid credentials" />
      </I18nProvider>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('invalid credentials');
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```bash
cd web
pnpm test -- components/LoginForm.test.tsx
cd ..
```

Expected: failure because `LoginForm` does not exist.

- [ ] **Step 3: Implement the UI primitives**

Create `web/components/ui/Button.tsx`:

```tsx
import { forwardRef, type ButtonHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';
type Size = 'sm' | 'md';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

const variants: Record<Variant, string> = {
  primary: 'bg-sky-600 text-white hover:bg-sky-500 focus-visible:ring-sky-500',
  secondary:
    'border border-slate-300 bg-white text-slate-900 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:bg-slate-800',
  danger: 'bg-red-600 text-white hover:bg-red-500 focus-visible:ring-red-500',
  ghost: 'text-slate-700 hover:bg-slate-200 dark:text-slate-200 dark:hover:bg-slate-800',
};

const sizes: Record<Size, string> = {
  sm: 'h-8 px-3 text-sm',
  md: 'h-10 px-4 text-sm',
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { className, variant = 'primary', size = 'md', type = 'button', ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-lg font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus-visible:ring-offset-slate-950',
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
});
```

Create `web/components/ui/Input.tsx`:

```tsx
import { forwardRef, type InputHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...props }, ref) {
    return (
      <input
        ref={ref}
        className={cn(
          'h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 placeholder:text-slate-400 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/30 disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:disabled:bg-slate-800',
          className,
        )}
        {...props}
      />
    );
  },
);
```

Create `web/components/ui/Select.tsx`:

```tsx
import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className, children, ...props }, ref) {
    return (
      <select
        ref={ref}
        className={cn(
          'h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/30 disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:disabled:bg-slate-800',
          className,
        )}
        {...props}
      >
        {children}
      </select>
    );
  },
);
```

Create `web/components/ui/Switch.tsx`:

```tsx
'use client';

import { cn } from '@/lib/cn';

interface Props {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  disabled?: boolean;
}

export function Switch({ checked, onChange, label, disabled }: Props) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus-visible:ring-offset-slate-950',
        checked ? 'bg-sky-600' : 'bg-slate-300 dark:bg-slate-700',
      )}
    >
      <span
        className={cn(
          'inline-block h-4 w-4 transform rounded-full bg-white shadow transition',
          checked ? 'translate-x-6' : 'translate-x-1',
        )}
      />
    </button>
  );
}
```

Create `web/components/ui/Card.tsx`:

```tsx
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface Props {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function Card({ title, actions, children, className }: Props) {
  return (
    <section
      className={cn(
        'rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900',
        className,
      )}
    >
      {(title || actions) && (
        <header className="mb-4 flex items-center justify-between gap-3">
          {title ? <h2 className="text-base font-semibold">{title}</h2> : <span />}
          {actions}
        </header>
      )}
      {children}
    </section>
  );
}
```

Create `web/components/ui/Badge.tsx`:

```tsx
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

type Tone = 'neutral' | 'success' | 'warning' | 'danger' | 'info';

const tones: Record<Tone, string> = {
  neutral: 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200',
  success: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200',
  warning: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200',
  danger: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-200',
  info: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-200',
};

export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={cn('inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium', tones[tone])}>
      {children}
    </span>
  );
}
```

Create `web/components/ui/Banner.tsx`:

```tsx
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

type Tone = 'info' | 'warning' | 'danger' | 'success';

const tones: Record<Tone, string> = {
  info: 'border-sky-300 bg-sky-50 text-sky-900 dark:border-sky-800 dark:bg-sky-950/50 dark:text-sky-100',
  warning: 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950/50 dark:text-amber-100',
  danger: 'border-red-300 bg-red-50 text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-100',
  success:
    'border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-100',
};

export function Banner({ tone = 'info', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <div role={tone === 'danger' ? 'alert' : 'status'} className={cn('rounded-lg border px-4 py-3 text-sm', tones[tone])}>
      {children}
    </div>
  );
}
```

Create `web/components/ui/Dialog.tsx`:

```tsx
'use client';

import { useEffect, type ReactNode } from 'react';
import { Button } from './Button';

interface Props {
  open: boolean;
  title: string;
  children: ReactNode;
  onClose: () => void;
  onConfirm?: () => void;
  confirmLabel: string;
  cancelLabel: string;
  busy?: boolean;
}

export function Dialog({ open, title, children, onClose, onConfirm, confirmLabel, cancelLabel, busy }: Props) {
  useEffect(() => {
    if (!open) return;
    const listener = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', listener);
    return () => document.removeEventListener('keydown', listener);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/50 p-4">
      <div role="dialog" aria-modal="true" aria-label={title} className="w-full max-w-md rounded-xl bg-white p-5 shadow-xl dark:bg-slate-900">
        <h2 className="text-lg font-semibold">{title}</h2>
        <div className="mt-3 text-sm text-slate-600 dark:text-slate-300">{children}</div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {cancelLabel}
          </Button>
          {onConfirm && (
            <Button onClick={onConfirm} disabled={busy}>
              {confirmLabel}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
```

Create `web/components/ui/Skeleton.tsx`:

```tsx
import { cn } from '@/lib/cn';

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-slate-200 dark:bg-slate-800', className)} />;
}
```

Create `web/components/ui/Toast.tsx`:

```tsx
'use client';

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { cn } from '@/lib/cn';

type Kind = 'success' | 'error' | 'info';

interface ToastItem {
  id: number;
  message: string;
  kind: Kind;
}

interface ToastValue {
  toast: (message: string, kind?: Kind) => void;
}

const ToastContext = createContext<ToastValue | null>(null);

const styles: Record<Kind, string> = {
  success: 'border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-100',
  error: 'border-red-300 bg-red-50 text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100',
  info: 'border-sky-300 bg-sky-50 text-sky-900 dark:border-sky-800 dark:bg-sky-950 dark:text-sky-100',
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string, kind: Kind = 'info') => {
    const id = Date.now() + Math.random();
    setItems((current) => [...current, { id, message, kind }]);
    window.setTimeout(() => {
      setItems((current) => current.filter((item) => item.id !== id));
    }, 5000);
  }, []);

  const value = useMemo(() => ({ toast }), [toast]);
  return (
    <ToastContext.Provider value={value}>
      {children}
      <div aria-live="polite" className="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
        {items.map((item) => (
          <div key={item.id} className={cn('rounded-lg border px-4 py-3 text-sm shadow-lg', styles[item.kind])}>
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastValue {
  const value = useContext(ToastContext);
  if (!value) throw new Error('useToast must be used inside ToastProvider');
  return value;
}
```

- [ ] **Step 4: Implement the dirty-state guard and app shell**

Create `web/lib/dirty.tsx`:

```tsx
'use client';

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

interface DirtyValue {
  dirty: boolean;
  setDirty: (dirty: boolean) => void;
}

const DirtyContext = createContext<DirtyValue | null>(null);

export function DirtyProvider({ children }: { children: ReactNode }) {
  const [dirty, setDirty] = useState(false);
  const value = useMemo(() => ({ dirty, setDirty }), [dirty]);
  return <DirtyContext.Provider value={value}>{children}</DirtyContext.Provider>;
}

export function useDirty(): DirtyValue {
  const value = useContext(DirtyContext);
  if (!value) throw new Error('useDirty must be used inside DirtyProvider');
  return value;
}
```

Create `web/components/AppShell.tsx`:

```tsx
'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  Monitor,
  Moon,
  ScrollText,
  Settings,
  Sun,
  X,
} from 'lucide-react';
import { useEffect, useState, type MouseEvent, type ReactNode } from 'react';
import { Button } from '@/components/ui/Button';
import { Skeleton } from '@/components/ui/Skeleton';
import { useAuth } from '@/lib/auth';
import { useDirty } from '@/lib/dirty';
import { useI18n } from '@/lib/i18n';
import { useTheme } from '@/lib/theme';
import { cn } from '@/lib/cn';

export function AppShell({ children }: { children: ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { theme, cycle } = useTheme();
  const { username, loading, logout } = useAuth();
  const { dirty } = useDirty();
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const [online, setOnline] = useState(true);

  useEffect(() => {
    const up = () => setOnline(true);
    const down = () => setOnline(false);
    setOnline(navigator.onLine);
    window.addEventListener('online', up);
    window.addEventListener('offline', down);
    return () => {
      window.removeEventListener('online', up);
      window.removeEventListener('offline', down);
    };
  }, []);

  useEffect(() => {
    if (!loading && !username) window.location.href = '/login';
  }, [loading, username]);

  useEffect(() => {
    setMenuOpen(false);
  }, [pathname]);

  if (loading) {
    return (
      <div className="space-y-4 p-8">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  if (!username) return null;

  const title = pathname.startsWith('/config')
    ? t('config.title')
    : pathname.startsWith('/logs')
      ? t('logs.title')
      : t('dashboard.title');

  const links = [
    { href: '/', label: t('nav.dashboard'), icon: LayoutDashboard },
    { href: '/config/', label: t('nav.config'), icon: Settings },
    { href: '/logs/', label: t('nav.logs'), icon: ScrollText },
  ];

  const guard = (event: MouseEvent) => {
    if (dirty && !window.confirm(t('config.unsaved_confirm'))) {
      event.preventDefault();
    }
  };

  const themeIcon = theme === 'dark' ? <Moon size={18} /> : theme === 'light' ? <Sun size={18} /> : <Monitor size={18} />;

  return (
    <div className="min-h-screen lg:flex">
      <aside
        className={cn(
          'fixed inset-y-0 left-0 z-40 w-64 transform border-r border-slate-200 bg-white p-4 transition-transform dark:border-slate-800 dark:bg-slate-900 lg:static lg:translate-x-0',
          menuOpen ? 'translate-x-0' : '-translate-x-full',
        )}
      >
        <div className="mb-6 flex items-center justify-between">
          <Link href="/" className="text-lg font-bold tracking-tight" onClick={guard}>
            {t('app.name')}
          </Link>
          <button className="lg:hidden" onClick={() => setMenuOpen(false)} aria-label={t('common.close')}>
            <X size={20} />
          </button>
        </div>
        <nav className="space-y-1">
          {links.map(({ href, label, icon: Icon }) => {
            const active = href === '/' ? pathname === '/' : pathname.startsWith(href.replace(/\/$/, ''));
            return (
              <Link
                key={href}
                href={href}
                onClick={guard}
                className={cn(
                  'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition',
                  active
                    ? 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-200'
                    : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800',
                )}
              >
                <Icon size={18} />
                {label}
              </Link>
            );
          })}
        </nav>
        <div className="mt-6 border-t border-slate-200 pt-4 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
          {username}
        </div>
      </aside>

      <div className="min-w-0 flex-1">
        <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-slate-200 bg-white/90 px-4 backdrop-blur dark:border-slate-800 dark:bg-slate-950/90">
          <button className="lg:hidden" onClick={() => setMenuOpen(true)} aria-label={t('nav.dashboard')}>
            <Menu size={22} />
          </button>
          <h1 className="mr-auto text-lg font-semibold">{title}</h1>
          <span className={cn('hidden items-center gap-1.5 text-xs sm:flex', online ? 'text-emerald-600' : 'text-red-600')}>
            <span className={cn('h-2 w-2 rounded-full', online ? 'bg-emerald-500' : 'bg-red-500')} />
            {online ? t('top.connected') : t('top.disconnected')}
          </span>
          <Button variant="ghost" size="sm" onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')} aria-label={t('top.language')}>
            <Languages size={18} />
            {lang === 'zh' ? 'EN' : '中文'}
          </Button>
          <Button variant="ghost" size="sm" onClick={cycle} aria-label={t('top.theme')}>
            {themeIcon}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={async () => {
              await logout();
              window.location.href = '/login';
            }}
            aria-label={t('nav.logout')}
          >
            <LogOut size={18} />
          </Button>
        </header>
        <main className="mx-auto max-w-6xl p-4 sm:p-6">{children}</main>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Implement the login form and page**

Create `web/components/LoginForm.tsx`:

```tsx
'use client';

import { useState, type FormEvent } from 'react';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Banner } from '@/components/ui/Banner';
import { useI18n } from '@/lib/i18n';

interface Props {
  onSubmit: (username: string, password: string) => Promise<void>;
  error?: string;
}

export function LoginForm({ onSubmit, error }: Props) {
  const { t } = useI18n();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      await onSubmit(username, password);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      {error && <Banner tone="danger">{error}</Banner>}
      <label className="block space-y-1.5">
        <span className="text-sm font-medium">{t('login.username')}</span>
        <Input
          autoComplete="username"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          required
        />
      </label>
      <label className="block space-y-1.5">
        <span className="text-sm font-medium">{t('login.password')}</span>
        <Input
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />
      </label>
      <Button type="submit" className="w-full" disabled={submitting}>
        {submitting ? t('common.loading') : t('login.submit')}
      </Button>
    </form>
  );
}
```

The test uses `getByLabelText(/username/i)`; the label wraps the input with a span text, so it associates. Good.

Create `web/app/(auth)/login/page.tsx`:

```tsx
'use client';

import { useEffect, useState } from 'react';
import { LoginForm } from '@/components/LoginForm';
import { Card } from '@/components/ui/Card';
import { errorMessage, useI18n } from '@/lib/i18n';
import { useAuth } from '@/lib/auth';

export default function LoginPage() {
  const { t } = useI18n();
  const { login, username, loading } = useAuth();
  const [error, setError] = useState('');

  useEffect(() => {
    if (!loading && username) window.location.href = '/';
  }, [loading, username]);

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 p-4 dark:bg-slate-950">
      <Card className="w-full max-w-sm">
        <div className="mb-6 space-y-1">
          <h1 className="text-xl font-semibold">{t('login.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('login.subtitle')}</p>
        </div>
        <LoginForm
          error={error}
          onSubmit={async (user, password) => {
            setError('');
            try {
              await login(user, password);
              window.location.href = '/';
            } catch (err) {
              setError(errorMessage(t, err));
            }
          }}
        />
      </Card>
    </main>
  );
}
```

Create `web/app/(app)/layout.tsx`:

```tsx
import type { ReactNode } from 'react';
import { AppShell } from '@/components/AppShell';

export default function AppLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
```

- [ ] **Step 6: Update providers**

Replace `web/app/providers.tsx` with:

```tsx
'use client';

import type { ReactNode } from 'react';
import { ToastProvider } from '@/components/ui/Toast';
import { AuthProvider } from '@/lib/auth';
import { DirtyProvider } from '@/lib/dirty';
import { I18nProvider } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme';

export function Providers({ children }: { children: ReactNode }) {
  return (
    <I18nProvider>
      <ThemeProvider>
        <AuthProvider>
          <DirtyProvider>
            <ToastProvider>{children}</ToastProvider>
          </DirtyProvider>
        </AuthProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
```

- [ ] **Step 7: Run tests and typecheck**

```bash
cd web
pnpm test
pnpm exec tsc --noEmit
cd ..
```

Expected: `LoginForm` tests pass, and typecheck passes.

- [ ] **Step 8: Commit checkpoint**

```bash
git add web
git commit -m "feat(web): add UI primitives, app shell, login, and dirty guard"
```

Only run if the user has authorized commits.

## Task 12: Dashboard, Configuration Form, and Live Logs Pages

**Files:**
- Create: `web/components/config/Field.tsx`
- Create: `web/components/config/ConfigForm.tsx`
- Create: `web/components/config/ConfigForm.test.tsx`
- Create: `web/app/(app)/page.tsx`
- Create: `web/app/(app)/config/page.tsx`
- Create: `web/app/(app)/logs/page.tsx`

**Interfaces:**
- Consumes: `api`, `useI18n`, `useToast`, `useDirty`, UI primitives, `Config`, `Status`, `InterfaceInfo`, `ApplyResult`, `LogLine`, `appendLog`, `recordGap`, `emptyLogState`, `restartSections`, `format*`
- Produces: the three authenticated pages and `ConfigForm`

- [ ] **Step 1: Write the failing ConfigForm test**

Create `web/components/config/ConfigForm.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { I18nProvider } from '@/lib/i18n';
import type { Config } from '@/lib/types';
import { ConfigForm } from './ConfigForm';

const value: Config = {
  proxy: { type: 'socks5', listen_address: '0.0.0.0', listen_port: 1080, username: 'admin', password: '' },
  network: {
    interface_name: 'eth0',
    ipv4_enabled: false,
    ipv6_enabled: true,
    ipv6_subnet: '2001:db8::/64',
    dns_source_address: '',
    rotate_ipv6: true,
    ipv6_pool_size: 64,
    ipv6_max_usage: 100,
    ipv6_max_age: 30,
  },
  monitoring: { enabled: false, port: 9090, allow_remote: false },
  logging: { debug_level: 1 },
  web: {
    enabled: true,
    listen_address: '127.0.0.1',
    port: 18081,
    username: 'admin',
    password: '',
    tls_cert_file: '',
    tls_key_file: '',
    trust_proxy_headers: false,
    cookie_secure: false,
  },
};

describe('ConfigForm', () => {
  it('disables locked fields and shows field errors', () => {
    render(
      <I18nProvider>
        <ConfigForm
          value={value}
          locked={{ 'network.ipv6_subnet': 'env', 'web.port': 'restart_required' }}
          errors={{ 'proxy.listen_port': 'must be between 1 and 65535' }}
          interfaces={[]}
          passwordsSet={{ 'proxy.password': true, 'web.password': true }}
          onChange={vi.fn()}
        />
      </I18nProvider>,
    );

    expect(screen.getByLabelText('IPv6 /64 subnet')).toBeDisabled();
    expect(screen.getByLabelText('Console port')).toBeDisabled();
    expect(screen.getByText('must be between 1 and 65535')).toBeInTheDocument();
    expect(screen.getByText('Managed by environment')).toBeInTheDocument();
    expect(screen.getByText('Restart required')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```bash
cd web
pnpm test -- components/config/ConfigForm.test.tsx
cd ..
```

Expected: failure because `ConfigForm` and `Field` do not exist.

- [ ] **Step 3: Implement the form field wrapper and configuration form**

Create `web/components/config/Field.tsx`:

```tsx
import type { ReactNode } from 'react';
import { Badge } from '@/components/ui/Badge';

interface Props {
  id?: string;
  label: string;
  error?: string;
  locked?: 'env' | 'restart_required';
  lockLabel?: string;
  children: ReactNode;
}

export function Field({ id, label, error, locked, lockLabel, children }: Props) {
  return (
    <div className="space-y-1.5">
      <div className="flex min-h-5 items-center justify-between gap-2">
        <label htmlFor={id} className="text-sm font-medium">
          {label}
        </label>
        {locked && <Badge tone={locked === 'env' ? 'warning' : 'neutral'}>{lockLabel}</Badge>}
      </div>
      {children}
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
```

Create `web/components/config/ConfigForm.tsx`:

```tsx
'use client';

import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { Switch } from '@/components/ui/Switch';
import { Card } from '@/components/ui/Card';
import { useI18n } from '@/lib/i18n';
import type { Config, InterfaceInfo } from '@/lib/types';
import { Field } from './Field';

interface Props {
  value: Config;
  locked: Record<string, string>;
  errors: Record<string, string>;
  interfaces: InterfaceInfo[];
  passwordsSet: Record<string, boolean>;
  onChange: (next: Config) => void;
}

export function ConfigForm({ value, locked, errors, interfaces, passwordsSet, onChange }: Props) {
  const { t } = useI18n();

  const lock = (field: string) => locked[field] as 'env' | 'restart_required' | undefined;
  const lockLabel = (field: string) =>
    lock(field) === 'env' ? t('config.locked_env') : t('config.locked_restart');
  const isDisabled = (field: string) => Boolean(lock(field));

  return (
    <div className="space-y-6">
      <Card title={t('config.proxy')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="proxy.type" label={t('config.field.proxy.type')} error={errors['proxy.type']}>
            <Select
              id="proxy.type"
              value={value.proxy.type}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, type: event.target.value as 'socks5' | 'https' } })
              }
            >
              <option value="socks5">SOCKS5</option>
              <option value="https">HTTPS</option>
            </Select>
          </Field>
          <Field
            id="proxy.listen_port"
            label={t('config.field.proxy.listen_port')}
            error={errors['proxy.listen_port']}
          >
            <Input
              id="proxy.listen_port"
              type="number"
              min={1}
              max={65535}
              value={value.proxy.listen_port}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, listen_port: Number(event.target.value) } })
              }
            />
          </Field>
          <Field
            id="proxy.listen_address"
            label={t('config.field.proxy.listen_address')}
            error={errors['proxy.listen_address']}
          >
            <Input
              id="proxy.listen_address"
              value={value.proxy.listen_address}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, listen_address: event.target.value } })
              }
            />
          </Field>
          <Field
            id="proxy.username"
            label={t('config.field.proxy.username')}
            error={errors['proxy.username']}
          >
            <Input
              id="proxy.username"
              value={value.proxy.username}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, username: event.target.value } })
              }
            />
          </Field>
          <Field
            id="proxy.password"
            label={t('config.field.proxy.password')}
            error={errors['proxy.password']}
          >
            <Input
              id="proxy.password"
              type="password"
              autoComplete="new-password"
              placeholder={passwordsSet['proxy.password'] ? t('config.password_keep') : undefined}
              value={value.proxy.password}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, password: event.target.value } })
              }
            />
          </Field>
        </div>
      </Card>

      <Card title={t('config.network')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            id="network.interface_name"
            label={t('config.field.network.interface_name')}
            error={errors['network.interface_name']}
            locked={lock('network.interface_name')}
            lockLabel={lockLabel('network.interface_name')}
          >
            <Select
              id="network.interface_name"
              disabled={isDisabled('network.interface_name')}
              value={value.network.interface_name}
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, interface_name: event.target.value } })
              }
            >
              <option value="">auto</option>
              {interfaces.map((iface) => (
                <option key={iface.name} value={iface.name}>
                  {iface.name}
                  {iface.suggested_prefix ? ` — ${iface.suggested_prefix}` : ''}
                </option>
              ))}
              {value.network.interface_name &&
                !interfaces.some((iface) => iface.name === value.network.interface_name) && (
                  <option value={value.network.interface_name}>{value.network.interface_name}</option>
                )}
            </Select>
          </Field>
          <Field
            id="network.ipv6_subnet"
            label={t('config.field.network.ipv6_subnet')}
            error={errors['network.ipv6_subnet']}
            locked={lock('network.ipv6_subnet')}
            lockLabel={lockLabel('network.ipv6_subnet')}
          >
            <Input
              id="network.ipv6_subnet"
              disabled={isDisabled('network.ipv6_subnet')}
              value={value.network.ipv6_subnet}
              placeholder="2001:db8::/64"
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, ipv6_subnet: event.target.value } })
              }
            />
          </Field>
          <Field
            id="network.dns_source_address"
            label={t('config.field.network.dns_source_address')}
            error={errors['network.dns_source_address']}
          >
            <Input
              id="network.dns_source_address"
              value={value.network.dns_source_address}
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, dns_source_address: event.target.value } })
              }
            />
          </Field>
          <div className="flex items-end gap-6">
            <div className="space-y-2">
              <span className="block text-sm font-medium">{t('config.field.network.ipv4_enabled')}</span>
              <Switch
                label={t('config.field.network.ipv4_enabled')}
                checked={value.network.ipv4_enabled}
                onChange={(checked) => onChange({ ...value, network: { ...value.network, ipv4_enabled: checked } })}
              />
            </div>
            <div className="space-y-2">
              <span className="block text-sm font-medium">{t('config.field.network.ipv6_enabled')}</span>
              <Switch
                label={t('config.field.network.ipv6_enabled')}
                checked={value.network.ipv6_enabled}
                onChange={(checked) => onChange({ ...value, network: { ...value.network, ipv6_enabled: checked } })}
              />
            </div>
          </div>
        </div>
      </Card>

      <Card title={t('config.rotation')}>
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-medium">{t('config.field.network.rotate_ipv6')}</span>
            <Switch
              label={t('config.field.network.rotate_ipv6')}
              checked={value.network.rotate_ipv6}
              onChange={(checked) => onChange({ ...value, network: { ...value.network, rotate_ipv6: checked } })}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field
              id="network.ipv6_pool_size"
              label={t('config.field.network.ipv6_pool_size')}
              error={errors['network.ipv6_pool_size']}
            >
              <Input
                id="network.ipv6_pool_size"
                type="number"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_pool_size}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_pool_size: Number(event.target.value) } })
                }
              />
            </Field>
            <Field
              id="network.ipv6_max_usage"
              label={t('config.field.network.ipv6_max_usage')}
              error={errors['network.ipv6_max_usage']}
            >
              <Input
                id="network.ipv6_max_usage"
                type="number"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_max_usage}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_max_usage: Number(event.target.value) } })
                }
              />
            </Field>
            <Field
              id="network.ipv6_max_age"
              label={t('config.field.network.ipv6_max_age')}
              error={errors['network.ipv6_max_age']}
            >
              <Input
                id="network.ipv6_max_age"
                type="number"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_max_age}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_max_age: Number(event.target.value) } })
                }
              />
            </Field>
          </div>
        </div>
      </Card>

      <Card title={t('config.monitoring')}>
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-medium">{t('config.field.monitoring.enabled')}</span>
            <Switch
              label={t('config.field.monitoring.enabled')}
              checked={value.monitoring.enabled}
              onChange={(checked) => onChange({ ...value, monitoring: { ...value.monitoring, enabled: checked } })}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              id="monitoring.port"
              label={t('config.field.monitoring.port')}
              error={errors['monitoring.port']}
            >
              <Input
                id="monitoring.port"
                type="number"
                min={1}
                max={65535}
                disabled={!value.monitoring.enabled}
                value={value.monitoring.port}
                onChange={(event) =>
                  onChange({ ...value, monitoring: { ...value.monitoring, port: Number(event.target.value) } })
                }
              />
            </Field>
            <div className="flex items-end gap-3">
              <span className="text-sm font-medium">{t('config.field.monitoring.allow_remote')}</span>
              <Switch
                label={t('config.field.monitoring.allow_remote')}
                checked={value.monitoring.allow_remote}
                disabled={!value.monitoring.enabled}
                onChange={(checked) =>
                  onChange({ ...value, monitoring: { ...value.monitoring, allow_remote: checked } })
                }
              />
            </div>
          </div>
        </div>
      </Card>

      <Card title={t('config.logging')}>
        <Field
          id="logging.debug_level"
          label={t('config.field.logging.debug_level')}
          error={errors['logging.debug_level']}
        >
          <Input
            id="logging.debug_level"
            type="number"
            min={0}
            max={2}
            value={value.logging.debug_level}
            onChange={(event) =>
              onChange({ ...value, logging: { ...value.logging, debug_level: Number(event.target.value) } })
            }
          />
        </Field>
      </Card>

      <Card title={t('config.web')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{t('config.field.web.enabled')}</span>
              {lock('web.enabled') && (
                <span className="text-xs text-slate-500 dark:text-slate-400">{lockLabel('web.enabled')}</span>
              )}
            </div>
            <Switch
              label={t('config.field.web.enabled')}
              checked={value.web.enabled}
              disabled={isDisabled('web.enabled')}
              onChange={(checked) => onChange({ ...value, web: { ...value.web, enabled: checked } })}
            />
          </div>
          <Field
            id="web.port"
            label={t('config.field.web.port')}
            error={errors['web.port']}
            locked={lock('web.port')}
            lockLabel={lockLabel('web.port')}
          >
            <Input
              id="web.port"
              type="number"
              min={1}
              max={65535}
              disabled={isDisabled('web.port')}
              value={value.web.port}
              onChange={(event) => onChange({ ...value, web: { ...value.web, port: Number(event.target.value) } })}
            />
          </Field>
          <Field
            id="web.listen_address"
            label={t('config.field.web.listen_address')}
            error={errors['web.listen_address']}
            locked={lock('web.listen_address')}
            lockLabel={lockLabel('web.listen_address')}
          >
            <Input
              id="web.listen_address"
              disabled={isDisabled('web.listen_address')}
              value={value.web.listen_address}
              onChange={(event) =>
                onChange({ ...value, web: { ...value.web, listen_address: event.target.value } })
              }
            />
          </Field>
          <Field
            id="web.username"
            label={t('config.field.web.username')}
            error={errors['web.username']}
          >
            <Input
              id="web.username"
              value={value.web.username}
              onChange={(event) => onChange({ ...value, web: { ...value.web, username: event.target.value } })}
            />
          </Field>
          <Field
            id="web.password"
            label={t('config.field.web.password')}
            error={errors['web.password']}
          >
            <Input
              id="web.password"
              type="password"
              autoComplete="new-password"
              placeholder={passwordsSet['web.password'] ? t('config.password_keep') : undefined}
              value={value.web.password}
              onChange={(event) => onChange({ ...value, web: { ...value.web, password: event.target.value } })}
            />
          </Field>
        </div>

        <details className="mt-5 rounded-lg border border-slate-200 p-4 dark:border-slate-800">
          <summary className="cursor-pointer text-sm font-medium">{t('config.advanced')}</summary>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <Field
              id="web.tls_cert_file"
              label={t('config.field.web.tls_cert_file')}
              error={errors['web.tls_cert_file']}
              locked={lock('web.tls_cert_file')}
              lockLabel={lockLabel('web.tls_cert_file')}
            >
              <Input
                id="web.tls_cert_file"
                disabled={isDisabled('web.tls_cert_file')}
                value={value.web.tls_cert_file}
                onChange={(event) =>
                  onChange({ ...value, web: { ...value.web, tls_cert_file: event.target.value } })
                }
              />
            </Field>
            <Field
              id="web.tls_key_file"
              label={t('config.field.web.tls_key_file')}
              error={errors['web.tls_key_file']}
              locked={lock('web.tls_key_file')}
              lockLabel={lockLabel('web.tls_key_file')}
            >
              <Input
                id="web.tls_key_file"
                disabled={isDisabled('web.tls_key_file')}
                value={value.web.tls_key_file}
                onChange={(event) =>
                  onChange({ ...value, web: { ...value.web, tls_key_file: event.target.value } })
                }
              />
            </Field>
            <div className="flex items-end gap-3">
              <span className="text-sm font-medium">{t('config.field.web.trust_proxy_headers')}</span>
              <Switch
                label={t('config.field.web.trust_proxy_headers')}
                checked={value.web.trust_proxy_headers}
                disabled={isDisabled('web.trust_proxy_headers')}
                onChange={(checked) =>
                  onChange({ ...value, web: { ...value.web, trust_proxy_headers: checked } })
                }
              />
            </div>
            <div className="flex items-end gap-3">
              <span className="text-sm font-medium">{t('config.field.web.cookie_secure')}</span>
              <Switch
                label={t('config.field.web.cookie_secure')}
                checked={value.web.cookie_secure}
                disabled={isDisabled('web.cookie_secure')}
                onChange={(checked) => onChange({ ...value, web: { ...value.web, cookie_secure: checked } })}
              />
            </div>
          </div>
        </details>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Implement the dashboard**

Create `web/app/(app)/page.tsx`:

```tsx
'use client';

import { useCallback, useEffect, useState } from 'react';
import { Banner } from '@/components/ui/Banner';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Skeleton } from '@/components/ui/Skeleton';
import { api } from '@/lib/api';
import { errorMessage, useI18n } from '@/lib/i18n';
import type { Status } from '@/lib/types';
import { formatBytes, formatDate, formatDuration } from '@/lib/format';

const stateTone = {
  running: 'success',
  starting: 'info',
  error: 'danger',
  stopped: 'neutral',
} as const;

export default function DashboardPage() {
  const { t, lang } = useI18n();
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [retrying, setRetrying] = useState(false);

  const load = useCallback(async () => {
    try {
      setStatus(await api<Status>('/api/status'));
      setError('');
    } catch (err) {
      setError(errorMessage(t, err));
    }
  }, [t]);

  useEffect(() => {
    void load();
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load();
    }, 5000);
    const visibility = () => {
      if (document.visibilityState === 'visible') void load();
    };
    document.addEventListener('visibilitychange', visibility);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [load]);

  const retry = async () => {
    setRetrying(true);
    try {
      await api('/api/proxy/retry', { method: 'POST' });
      await load();
    } catch (err) {
      setError(errorMessage(t, err));
    } finally {
      setRetrying(false);
    }
  };

  if (!status) {
    return (
      <div className="space-y-4">
        {error && <Banner tone="danger">{error}</Banner>}
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  const proxy = status.proxy;
  const stateLabel = t(`dashboard.state.${proxy.state}` as 'dashboard.state.running');

  return (
    <div className="space-y-4">
      {status.apply.in_progress && (
        <Banner tone="info">
          {t('dashboard.apply_in_progress', { step: status.apply.step || t('config.applying') })}
        </Banner>
      )}
      {status.platform.warnings.map((warning) => (
        <Banner key={warning} tone="warning">
          {warning}
        </Banner>
      ))}
      {error && <Banner tone="danger">{error}</Banner>}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card
          title={t('dashboard.proxy')}
          actions={<Badge tone={stateTone[proxy.state]}>{stateLabel}</Badge>}
        >
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Row label={t('dashboard.type')} value={proxy.type} />
            <Row label={t('dashboard.listen')} value={proxy.listen || '—'} />
            <Row label={t('dashboard.protocol')} value={`IPv${proxy.protocol}`} />
            <Row label={t('dashboard.uptime')} value={formatDuration(status.uptime_seconds)} />
            <Row
              label={t('dashboard.monitoring')}
              value={status.monitoring.enabled ? status.monitoring.state : t('common.disabled')}
            />
          </dl>
          {proxy.error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{proxy.error}</p>}
          {proxy.state === 'error' && (
            <Button className="mt-4" onClick={retry} disabled={retrying}>
              {retrying ? t('common.loading') : t('common.retry')}
            </Button>
          )}
        </Card>

        <Card title={t('dashboard.pool')}>
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Row label={t('common.enabled')} value={status.pool.enabled ? t('common.enabled') : t('common.disabled')} />
            <Row label={t('dashboard.pool_size')} value={String(status.pool.size)} />
            <Row label={t('dashboard.prefix')} value={status.pool.prefix || '—'} />
            <Row label={t('dashboard.rotations')} value={String(status.pool.rotations)} />
            <Row label={t('dashboard.max_usage')} value={String(status.pool.max_usage)} />
            <Row label={t('dashboard.max_age')} value={`${status.pool.max_age_minutes}m`} />
          </dl>
        </Card>

        <Card title={t('dashboard.traffic')}>
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Row label={t('dashboard.active_connections')} value={String(status.metrics.active_connections)} />
            <Row label={t('dashboard.total_connections')} value={String(status.metrics.total_connections)} />
            <Row label={t('dashboard.requests')} value={String(status.metrics.total_requests)} />
            <Row label={t('dashboard.bytes_sent')} value={formatBytes(status.metrics.bytes_sent)} />
            <Row label={t('dashboard.bytes_received')} value={formatBytes(status.metrics.bytes_received)} />
          </dl>
        </Card>

        <Card title={t('dashboard.config')}>
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Row label={t('dashboard.generation')} value={String(status.config.generation)} />
            <Row label={t('dashboard.last_applied')} value={formatDate(status.config.last_applied_at, lang)} />
            <Row label={t('dashboard.config_path')} value={status.config.path} />
            <Row
              label={t('dashboard.last_applied')}
              value={
                status.config.last_apply.ok
                  ? t('config.last_ok', { generation: status.config.last_apply.generation })
                  : status.config.last_apply.error || '—'
              }
            />
          </dl>
        </Card>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="mt-0.5 break-all font-medium">{value}</dd>
    </div>
  );
}
```

- [ ] **Step 5: Implement the configuration page**

Create `web/app/(app)/config/page.tsx`:

```tsx
'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { Banner } from '@/components/ui/Banner';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Dialog } from '@/components/ui/Dialog';
import { Skeleton } from '@/components/ui/Skeleton';
import { useToast } from '@/components/ui/Toast';
import { ConfigForm } from '@/components/config/ConfigForm';
import { api, ApiError } from '@/lib/api';
import { hasChanges, restartSections } from '@/lib/config-diff';
import { validateConfig } from '@/lib/validate';
import { useDirty } from '@/lib/dirty';
import { errorMessage, useI18n, type MessageKey } from '@/lib/i18n';
import type { ApplyResult, Config, ConfigResponse, InterfaceInfo, Meta, Status } from '@/lib/types';

function redactPasswords(cfg: Config): Config {
  return {
    ...cfg,
    proxy: { ...cfg.proxy, password: '' },
    web: { ...cfg.web, password: '' },
  };
}

export default function ConfigPage() {
  const { t } = useI18n();
  const tRef = useRef(t);
  useEffect(() => {
    tRef.current = t;
  }, [t]);
  const { toast } = useToast();
  const { setDirty } = useDirty();

  const [form, setForm] = useState<Config | null>(null);
  const [original, setOriginal] = useState<Config | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState('');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [lastApply, setLastApply] = useState<Status['config']['last_apply'] | null>(null);

  const dirty = Boolean(form && original && hasChanges(original, form));

  useEffect(() => {
    setDirty(dirty);
    return () => setDirty(false);
  }, [dirty, setDirty]);

  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);

  const load = useCallback(async () => {
    try {
      const [configResponse, ifaces, status] = await Promise.all([
        api<ConfigResponse>('/api/config'),
        api<InterfaceInfo[]>('/api/network/interfaces'),
        api<Status>('/api/status'),
      ]);
      setForm(configResponse.config);
      setOriginal(configResponse.config);
      setMeta(configResponse.meta);
      setInterfaces(ifaces);
      setLastApply(status.config.last_apply);
      setErrors({});
      setError('');
    } catch (err) {
      setError(errorMessage(tRef.current, err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const apply = async () => {
    if (!form || !original) return;
    const clientErrors = validateConfig(form, meta?.passwords_set ?? {});
    if (Object.keys(clientErrors).length > 0) {
      setConfirmOpen(false);
      setErrors(clientErrors);
      setError(t('config.invalid'));
      return;
    }
    setConfirmOpen(false);
    setSaving(true);
    setErrors({});
    setError('');
    const poll = window.setInterval(async () => {
      try {
        const status = await api<Status>('/api/status');
        setStep(status.apply.step);
      } catch {
        // Ignore transient polling errors while the apply request is running.
      }
    }, 1000);

    try {
      const result = await api<ApplyResult>('/api/config', {
        method: 'PUT',
        body: JSON.stringify(form),
      });
      const saved = redactPasswords(form);
      const nextPasswordsSet = {
        ...(meta?.passwords_set ?? {}),
        'proxy.password':
          form.proxy.password !== '' || Boolean(meta?.passwords_set?.['proxy.password']),
        'web.password': form.web.password !== '' || Boolean(meta?.passwords_set?.['web.password']),
      };
      setForm(saved);
      setOriginal(saved);
      setMeta((current) =>
        current
          ? { ...current, generation: result.generation, passwords_set: nextPasswordsSet }
          : current,
      );
      setLastApply({ ok: true, error: '', at: new Date().toISOString(), generation: result.generation });
      toast(t('config.saved'), 'success');
      if (result.reauth_required) {
        window.location.href = '/login';
        return;
      }
    } catch (err) {
      if (err instanceof ApiError && err.fields) {
        setErrors(err.fields);
      }
      const message = errorMessage(t, err);
      setError(message);
      setLastApply({ ok: false, error: message, at: new Date().toISOString(), generation: meta?.generation ?? 0 });
      toast(t('config.failed'), 'error');
    } finally {
      window.clearInterval(poll);
      setSaving(false);
      setStep('');
    }
  };

  const save = () => {
    if (!form || !original) return;
    if (restartSections(original, form).length > 0) {
      setConfirmOpen(true);
      return;
    }
    void apply();
  };

  if (!form || !meta) {
    return (
      <div className="space-y-4">
        {error && <Banner tone="danger">{error}</Banner>}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const stepLabel = step ? t(`config.step.${step}` as MessageKey) : t('config.applying');

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        {dirty && <Banner tone="warning">{t('config.unsaved')}</Banner>}
        {saving && <Banner tone="info">{stepLabel}</Banner>}
        {!saving && lastApply && (
          <Banner tone={lastApply.ok ? 'success' : 'danger'}>
            {lastApply.ok
              ? t('config.last_ok', { generation: lastApply.generation })
              : t('config.last_failed', { error: lastApply.error })}
          </Banner>
        )}
        {!saving && error && <Banner tone="danger">{error}</Banner>}
      </div>

      <fieldset disabled={saving} className="space-y-4">
        <ConfigForm
          value={form}
          locked={meta.locked}
          errors={errors}
          interfaces={interfaces}
          passwordsSet={meta.passwords_set}
          onChange={setForm}
        />
      </fieldset>

      <Card>
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            variant="secondary"
            onClick={() => {
              if (dirty && !window.confirm(t('config.unsaved_confirm'))) return;
              void load();
            }}
            disabled={saving}
          >
            {t('common.reload')}
          </Button>
          <Button onClick={save} disabled={saving || !dirty}>
            {saving ? t('common.saving') : t('config.save')}
          </Button>
        </div>
      </Card>

      <Dialog
        open={confirmOpen}
        title={t('config.confirm_title')}
        confirmLabel={t('config.save')}
        cancelLabel={t('common.cancel')}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => void apply()}
        busy={saving}
      >
        {t('config.confirm_body')}
      </Dialog>
    </div>
  );
}
```

- [ ] **Step 6: Implement the logs page**

Create `web/app/(app)/logs/page.tsx`:

```tsx
'use client';

import { useEffect, useRef, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Banner } from '@/components/ui/Banner';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { api, ApiError } from '@/lib/api';
import { useI18n } from '@/lib/i18n';
import { appendLog, emptyLogState, recordGap, type LogState } from '@/lib/log-store';
import type { LogLine } from '@/lib/types';

export default function LogsPage() {
  const { t } = useI18n();
  const [state, setState] = useState<LogState>(emptyLogState());
  const [paused, setPaused] = useState(false);
  const [connected, setConnected] = useState(false);
  const [filter, setFilter] = useState('');
  const [newCount, setNewCount] = useState(0);
  const pausedRef = useRef(paused);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    const source = new EventSource('/api/logs/stream');
    source.onopen = () => setConnected(true);
    source.addEventListener('log', (event) => {
      const record = JSON.parse((event as MessageEvent).data) as LogLine;
      setState((current) => appendLog(current, record));
      if (pausedRef.current) setNewCount((count) => count + 1);
    });
    source.addEventListener('gap', (event) => {
      const data = JSON.parse((event as MessageEvent).data) as { dropped: number };
      setState((current) => recordGap(current, data.dropped));
    });
    source.onerror = () => {
      setConnected(false);
      void api('/api/auth/session').catch((err) => {
        if (err instanceof ApiError && err.status === 401) {
          source.close();
          window.location.href = '/login';
        }
      });
    };
    return () => source.close();
  }, []);

  useEffect(() => {
    if (paused || filter) return;
    bottomRef.current?.scrollIntoView({ block: 'end' });
    setNewCount(0);
  }, [state.lines, paused, filter]);

  const visible = filter
    ? state.lines.filter((line) => line.line.toLowerCase().includes(filter.toLowerCase()))
    : state.lines;

  return (
    <div className="space-y-4">
      {state.dropped > 0 && (
        <Banner tone="warning">{t('logs.gap', { count: state.dropped })}</Banner>
      )}
      <Card>
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone={connected ? 'success' : 'danger'}>
            {connected ? t('logs.connected') : t('logs.disconnected')}
          </Badge>
          <Input
            className="max-w-xs"
            placeholder={t('logs.filter')}
            aria-label={t('logs.filter')}
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
          <Button variant="secondary" size="sm" onClick={() => setPaused((value) => !value)}>
            {paused ? t('logs.resume') : t('logs.pause')}
          </Button>
          {paused && newCount > 0 && <Badge tone="info">{t('logs.new', { count: newCount })}</Badge>}
          <Button variant="ghost" size="sm" onClick={() => setState(emptyLogState())}>
            {t('logs.clear')}
          </Button>
        </div>
      </Card>

      <div className="h-[calc(100vh-16rem)] overflow-auto rounded-xl bg-slate-950 p-4 font-mono text-xs leading-5 text-slate-100 shadow-inner">
        {visible.length === 0 ? (
          <p className="text-slate-400">{t('logs.empty')}</p>
        ) : (
          visible.map((line) => (
            <div key={`${line.ts}-${line.seq}`} className="whitespace-pre-wrap break-all">
              <span className="mr-2 text-slate-500">{new Date(line.ts).toLocaleTimeString()}</span>
              {line.line}
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}
```

- [ ] **Step 7: Run tests, typecheck, and build the export**

```bash
cd web
pnpm test
pnpm exec tsc --noEmit
pnpm build
cd ..
```

Expected: tests pass, typecheck passes, and `web/out/` is produced. The build may warn about static export and client-only pages; those warnings are acceptable if the export completes.

- [ ] **Step 8: Commit checkpoint**

```bash
git add web
git commit -m "feat(web): add dashboard, configuration editor, and live log viewer"
```

Only run if the user has authorized commits.

## Task 13: Build Pipeline, Static Embedding, and Makefile

**Files:**
- Create: `scripts/build-web.sh`
- Create: `Makefile`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: `web/package.json`, `web/pnpm-lock.yaml`, `internal/webui/dist`
- Produces: `scripts/build-web.sh` copies `web/out` into `internal/webui/dist`; `make build` produces `dist/nyxproxy`.

- [ ] **Step 1: Create the frontend build script**

Create `scripts/build-web.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT/web"
if ! command -v pnpm >/dev/null 2>&1; then
  corepack enable
  corepack prepare pnpm@12.9.1 --activate
fi
pnpm install --frozen-lockfile
pnpm build

rm -rf "$ROOT/internal/webui/dist"
mkdir -p "$ROOT/internal/webui/dist"
cp -R out/. "$ROOT/internal/webui/dist/"
touch "$ROOT/internal/webui/dist/.gitkeep"

echo "web assets copied to internal/webui/dist"
```

Make it executable:

```bash
chmod +x scripts/build-web.sh
```

- [ ] **Step 2: Update `.gitignore`**

Append to `.gitignore`:

```gitignore
# Frontend dependencies and build output
web/node_modules/
web/.next/
web/out/
web/coverage/

# Embedded UI output (the .gitkeep file keeps the embed directory present)
internal/webui/dist/*
!internal/webui/dist/.gitkeep
```

- [ ] **Step 3: Create the Makefile**

Create `Makefile` (recipe lines must start with a tab):

```make
GO ?= go
PNPM ?= pnpm

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Version=$(VERSION) \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Commit=$(COMMIT) \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.BuildTime=$(BUILD_TIME)

.PHONY: web embed build test lint docker clean

web:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) build

embed: web
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -R web/out/. internal/webui/dist/
	touch internal/webui/dist/.gitkeep

build: embed
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/nyxproxy ./cmd/proxy

test:
	$(GO) test -race ./...
	cd web && $(PNPM) test

lint:
	$(GO) vet ./...
	cd web && $(PNPM) lint

docker:
	docker build -t nyxproxy-oss:local .

clean:
	rm -rf dist internal/webui/dist web/out web/.next
```

- [ ] **Step 4: Build the frontend and embed it**

```bash
cd web
pnpm install --frozen-lockfile
pnpm build
cd ..
rm -rf internal/webui/dist
mkdir -p internal/webui/dist
cp -R web/out/. internal/webui/dist/
touch internal/webui/dist/.gitkeep
```

Expected: `internal/webui/dist/index.html` exists.

- [ ] **Step 5: Verify the embedded build**

```bash
go build -trimpath -o dist/nyxproxy ./cmd/proxy
go test ./internal/webui/... -count=1
```

Expected: the binary builds, and the webui tests still pass. The `Handler()` now finds `index.html` instead of the placeholder.

- [ ] **Step 6: Commit checkpoint**

```bash
git add scripts/build-web.sh Makefile .gitignore
git commit -m "build: add pnpm frontend pipeline, embed step, and Makefile"
```

Only run if the user has authorized commits.

---

## Task 14: Docker Entrypoint, Template, Compose, and Healthcheck

**Files:**
- Modify: `Dockerfile`
- Modify: `docker/entrypoint.sh`
- Modify: `docker/config.template.yaml`
- Modify: `.dockerignore`
- Modify: `.env.docker.example`
- Modify: `docker-compose.yml`
- Modify: `docker/docker-compose.example.yml`

**Interfaces:**
- Consumes: `scripts/build-web.sh` output, `nyxproxy -healthcheck -config /data/config.yaml`, `NYXPROXY_PREFIX`, `NYXPROXY_INTERFACE`
- Produces: a multi-arch image with the console embedded, `/data/config.yaml` persistence, and a healthcheck.

- [ ] **Step 1: Rewrite the Dockerfile**

Replace `Dockerfile` with:

```dockerfile
# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /app/web
RUN corepack enable && corepack prepare pnpm@12.9.1 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /app/web/out ./internal/webui/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Version=${VERSION} \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Commit=${COMMIT} \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.BuildTime=${BUILD_TIME}" \
    -o /out/nyxproxy ./cmd/proxy

FROM alpine:3.22

RUN apk add --no-cache iproute2 ca-certificates tzdata \
    && mkdir -p /data \
    && chmod 700 /data

COPY --from=builder /out/nyxproxy /usr/local/bin/nyxproxy
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/config.template.yaml /etc/nyxproxy/config.template.yaml

RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /data
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/usr/local/bin/nyxproxy", "-healthcheck", "-config", "/data/config.yaml"]

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

- [ ] **Step 2: Update the config template**

Replace `docker/config.template.yaml` with:

```yaml
proxy:
  type: socks5
  listen_address: 0.0.0.0
  listen_port: __PORT__
  username: __USER__
  password: __PASS__

network:
  interface_name: __IFACE__
  ipv4_enabled: false
  ipv6_enabled: true
  ipv6_subnet: __PREFIX__
  rotate_ipv6: true
  ipv6_pool_size: __POOL__
  ipv6_max_usage: __MAX_USAGE__
  ipv6_max_age: __MAX_AGE__
  dns_source_address: __DNS_SOURCE__

monitoring:
  enabled: false

logging:
  debug_level: __DEBUG_LEVEL__

web:
  enabled: __WEB_ENABLED__
  listen_address: __WEB_LISTEN__
  port: __WEB_PORT__
  username: __WEB_USER__
  password: __WEB_PASS__
  tls_cert_file: __WEB_TLS_CERT__
  tls_key_file: __WEB_TLS_KEY__
  trust_proxy_headers: __WEB_TRUST_PROXY__
  cookie_secure: __WEB_COOKIE_SECURE__
```

- [ ] **Step 3: Rewrite the entrypoint**

Replace `docker/entrypoint.sh` with:

```sh
#!/bin/sh
set -eu

CONFIG_PATH="${CONFIG_PATH:-/data/config.yaml}"
TEMPLATE="${TEMPLATE:-/etc/nyxproxy/config.template.yaml}"

IFACE="${IFACE:-}"
LISTEN_PORT="${LISTEN_PORT:-1080}"
POOL_SIZE="${POOL_SIZE:-64}"
MAX_USAGE="${MAX_USAGE:-100}"
MAX_AGE="${MAX_AGE:-30}"
PROXY_USER="${PROXY_USER:-admin}"
PROXY_PASS="${PROXY_PASS:-}"
DEBUG_LEVEL="${DEBUG_LEVEL:-1}"
DNS_SOURCE_ADDRESS="${DNS_SOURCE_ADDRESS:-}"
STATIC_PREFIX="${PREFIX:-}"

WEB_ENABLED="${WEB_ENABLED:-true}"
WEB_LISTEN="${WEB_LISTEN:-0.0.0.0}"
WEB_PORT="${WEB_PORT:-18081}"
WEB_USER="${WEB_USER:-admin}"
WEB_PASS="${WEB_PASS:-}"
WEB_TLS_CERT_FILE="${WEB_TLS_CERT_FILE:-}"
WEB_TLS_KEY_FILE="${WEB_TLS_KEY_FILE:-}"
WEB_TRUST_PROXY_HEADERS="${WEB_TRUST_PROXY_HEADERS:-false}"
WEB_COOKIE_SECURE="${WEB_COOKIE_SECURE:-false}"

log() {
	printf '[nyxproxy] %s\n' "$*"
}

fatal() {
	log "FATAL: $*"
	exit 1
}

bool_value() {
	case "$2" in
		true|false) printf '%s' "$2" ;;
		*) fatal "$1 must be true or false" ;;
	esac
}

if [ -z "$IFACE" ]; then
	IFACE="$(ip route show default 2>/dev/null | awk 'NR==1 {print $5}')"
fi
[ -n "$IFACE" ] || fatal "cannot determine the network interface; set IFACE"
[ -n "$PROXY_PASS" ] || fatal "PROXY_PASS is required"

WEB_ENABLED="$(bool_value WEB_ENABLED "$WEB_ENABLED")"
WEB_TRUST_PROXY_HEADERS="$(bool_value WEB_TRUST_PROXY_HEADERS "$WEB_TRUST_PROXY_HEADERS")"
WEB_COOKIE_SECURE="$(bool_value WEB_COOKIE_SECURE "$WEB_COOKIE_SECURE")"
if [ "$WEB_ENABLED" = "true" ] && { [ -z "$WEB_USER" ] || [ -z "$WEB_PASS" ]; }; then
	fatal "WEB_USER and WEB_PASS are required when WEB_ENABLED=true"
fi

prefix() {
	if [ -n "$STATIC_PREFIX" ]; then
		printf '%s' "$STATIC_PREFIX"
		return 0
	fi
	p="$(ip -6 route show dev "$IFACE" proto ra 2>/dev/null | awk '$1 ~ /\/64$/ {print $1; exit}')"
	if [ -z "$p" ]; then
		p="$(ip -6 route show dev "$IFACE" 2>/dev/null | awk '$1 ~ /\/64$/ {print $1; exit}')"
	fi
	printf '%s' "$p"
}

sed_escape() {
	printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

yaml_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

render_config() {
	p="$1"
	mkdir -p "$(dirname "$CONFIG_PATH")"
	chmod 700 "$(dirname "$CONFIG_PATH")"
	umask 077
	sed \
		-e "s|__PREFIX__|$(sed_escape "$(yaml_quote "$p")")|g" \
		-e "s|__PORT__|$(sed_escape "$LISTEN_PORT")|g" \
		-e "s|__USER__|$(sed_escape "$(yaml_quote "$PROXY_USER")")|g" \
		-e "s|__PASS__|$(sed_escape "$(yaml_quote "$PROXY_PASS")")|g" \
		-e "s|__IFACE__|$(sed_escape "$(yaml_quote "$IFACE")")|g" \
		-e "s|__POOL__|$(sed_escape "$POOL_SIZE")|g" \
		-e "s|__MAX_USAGE__|$(sed_escape "$MAX_USAGE")|g" \
		-e "s|__MAX_AGE__|$(sed_escape "$MAX_AGE")|g" \
		-e "s|__DEBUG_LEVEL__|$(sed_escape "$DEBUG_LEVEL")|g" \
		-e "s|__DNS_SOURCE__|$(sed_escape "$(yaml_quote "$DNS_SOURCE_ADDRESS")")|g" \
		-e "s|__WEB_ENABLED__|$(sed_escape "$WEB_ENABLED")|g" \
		-e "s|__WEB_LISTEN__|$(sed_escape "$(yaml_quote "$WEB_LISTEN")")|g" \
		-e "s|__WEB_PORT__|$(sed_escape "$WEB_PORT")|g" \
		-e "s|__WEB_USER__|$(sed_escape "$(yaml_quote "$WEB_USER")")|g" \
		-e "s|__WEB_PASS__|$(sed_escape "$(yaml_quote "$WEB_PASS")")|g" \
		-e "s|__WEB_TLS_CERT__|$(sed_escape "$(yaml_quote "$WEB_TLS_CERT_FILE")")|g" \
		-e "s|__WEB_TLS_KEY__|$(sed_escape "$(yaml_quote "$WEB_TLS_KEY_FILE")")|g" \
		-e "s|__WEB_TRUST_PROXY__|$(sed_escape "$WEB_TRUST_PROXY_HEADERS")|g" \
		-e "s|__WEB_COOKIE_SECURE__|$(sed_escape "$WEB_COOKIE_SECURE")|g" \
		"$TEMPLATE" > "$CONFIG_PATH"
	chmod 600 "$CONFIG_PATH"
}

child=""
shutting_down=0
backoff=2

cleanup() {
	shutting_down=1
	if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
		kill -TERM "$child" 2>/dev/null || true
		wait "$child" 2>/dev/null || true
	fi
	exit 0
}
trap cleanup TERM INT

wait_sleep() {
	sleep "$1" &
	wait $! || true
}

while :; do
	p="$(prefix)"
	if [ -z "$p" ]; then
		log "waiting for an IPv6 /64 on $IFACE (set PREFIX to override detection)"
		wait_sleep 5
		continue
	fi

	export NYXPROXY_PREFIX="$p"
	export NYXPROXY_INTERFACE="$IFACE"

	if [ ! -f "$CONFIG_PATH" ]; then
		log "seeding $CONFIG_PATH from environment"
		render_config "$p"
	fi

	log "starting nyxproxy with prefix $p (config: $CONFIG_PATH)"
	nyxproxy -config "$CONFIG_PATH" &
	child=$!
	current_prefix="$p"

	while kill -0 "$child" 2>/dev/null; do
		wait_sleep 10
		kill -0 "$child" 2>/dev/null || break
		now_prefix="$(prefix)"
		if [ "$now_prefix" != "$current_prefix" ]; then
			log "prefix changed from $current_prefix to ${now_prefix:-<none>}; restarting"
			kill -TERM "$child" 2>/dev/null || true
			wait "$child" 2>/dev/null || true
			child=""
			backoff=2
			break
		fi
	done

	if [ -n "$child" ]; then
		status=0
		wait "$child" || status=$?
		child=""
		if [ "$shutting_down" = "1" ]; then
			exit 0
		fi
		log "nyxproxy exited with status $status; restarting in ${backoff}s"
		wait_sleep "$backoff"
		backoff=$((backoff * 2))
		[ "$backoff" -le 10 ] || backoff=10
	fi
done
```

Make it executable:

```bash
chmod +x docker/entrypoint.sh
```

- [ ] **Step 4: Update `.dockerignore`**

Replace `.dockerignore` with:

```text
.git
.github
artifacts
dist
.cache
.env
.env.docker
.env.docker.example
config.yaml
*.exe
*.md
web/node_modules
web/.next
web/out
web/coverage
internal/webui/dist
docs
```

Keep the template and scripts inside the image; do not exclude `docker/`.

- [ ] **Step 5: Update the environment example**

Append to `.env.docker.example`:

```dotenv
# Web 管理面板（首次启动时写入 /data/config.yaml；之后请用面板修改）
WEB_ENABLED=true
WEB_LISTEN=0.0.0.0
WEB_PORT=18081
WEB_USER=admin
WEB_PASS=change_me_now
# 内置 TLS 证书（可选；容器内路径，挂载示例见 README）
# WEB_TLS_CERT_FILE=/certs/tls.crt
# WEB_TLS_KEY_FILE=/certs/tls.key
# 仅在可信反向代理之后开启
WEB_TRUST_PROXY_HEADERS=false
WEB_COOKIE_SECURE=false
```

- [ ] **Step 6: Add the persistent volume to both compose files**

In `docker-compose.yml`, add under the service:

```yaml
    volumes:
      - nyxproxy-data:/data
```

and at the end of the file:

```yaml
volumes:
  nyxproxy-data:
```

In `docker/docker-compose.example.yml`, add the same service volume and top-level volume. The service already uses `build.context: ..`; the path stays the same.

In both compose files, update the header comment to:

```text
# 编辑 .env.docker，设置 IFACE、PROXY_PASS 和 WEB_PASS
```

- [ ] **Step 7: Validate the shell script and compose YAML**

```bash
shellcheck docker/entrypoint.sh
```

Expected: no findings. If `shellcheck` is not installed locally, run it in CI (Task 15 adds the job).

Validate YAML with a local parser if available:

```bash
python -c "import yaml,sys; [yaml.safe_load(open(p)) for p in ['docker-compose.yml','docker/docker-compose.example.yml','docker/config.template.yaml']]"
```

Expected: no output and exit code 0. If Python is unavailable, use `ruby -e` or let CI run `docker compose config`.

- [ ] **Step 8: Commit checkpoint**

```bash
git add Dockerfile docker .dockerignore .env.docker.example docker-compose.yml
git commit -m "build(docker): embed the web console and persist config in /data"
```

Only run if the user has authorized commits.

## Task 15: CI, Release, Documentation, Example Config, and Windows Removal

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `.github/workflows/docker.yml`
- Modify: `README.md`
- Modify: `README-ZH.md`
- Modify: `CONTRIBUTING.md`
- Modify: `docs/CONFIG.md`
- Modify: `config.example.yaml`
- Modify: `scripts/quick-setup.sh`

**Interfaces:**
- Consumes: `web/pnpm-lock.yaml`, `scripts/build-web.sh`, `internal/webui/dist`
- Produces: pnpm-only CI, Linux-only releases, and complete English/Chinese console documentation.

- [ ] **Step 1: Rewrite the CI workflow**

Replace `.github/workflows/ci.yml` with:

```yaml
name: CI

on:
  push:
    branches:
      - main
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: pnpm/action-setup@v4
        with:
          version: 12.9.1

      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml

      - name: Install dependencies
        working-directory: web
        run: pnpm install --frozen-lockfile

      - name: Lint
        working-directory: web
        run: pnpm lint

      - name: Test
        working-directory: web
        run: pnpm test

      - name: Build static export
        working-directory: web
        run: pnpm build

      - uses: actions/upload-artifact@v4
        with:
          name: web-dist
          path: web/out
          if-no-files-found: error

  go:
    needs: web
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - uses: actions/download-artifact@v4
        with:
          name: web-dist
          path: internal/webui/dist

      - name: Go fmt (check)
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "gofmt needs to be run on:"
            echo "$unformatted"
            exit 1
          fi

      - name: Go vet
        run: go vet ./...

      - name: Go test (race)
        run: go test -race ./...

      - name: Build Linux binaries
        run: |
          mkdir -p dist
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/nyxproxy-linux-amd64 ./cmd/proxy
          CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/nyxproxy-linux-arm64 ./cmd/proxy

  shellcheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Shellcheck entrypoint and build script
        run: shellcheck docker/entrypoint.sh scripts/build-web.sh
```

If `actions/upload-artifact@v4` rejects hidden files, add `include-hidden-files: true`; static export output does not normally contain hidden files.

- [ ] **Step 2: Rewrite the release workflow**

Replace `.github/workflows/release.yml` with:

```yaml
name: Release NyxProxy-OSS

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  build-and-release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: pnpm/action-setup@v4
        with:
          version: 12.9.1

      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml

      - name: Build web console
        working-directory: web
        run: |
          pnpm install --frozen-lockfile
          pnpm build

      - name: Embed web console
        run: |
          rm -rf internal/webui/dist
          mkdir -p internal/webui/dist
          cp -R web/out/. internal/webui/dist/
          touch internal/webui/dist/.gitkeep

      - uses: actions/setup-go@v5
        with:
          go-version-file: "go.mod"
          cache: true

      - name: Download dependencies
        run: go mod download

      - name: Test
        run: go test -race ./...

      - name: Vet
        run: go vet ./...

      - name: Build for Linux
        env:
          VERSION: ${{ github.ref_name }}
          COMMIT: ${{ github.sha }}
        run: |
          mkdir -p dist
          BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

          BUILD_FLAGS="-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Version=${VERSION#v}"
          BUILD_FLAGS="${BUILD_FLAGS} -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Commit=${COMMIT}"
          BUILD_FLAGS="${BUILD_FLAGS} -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.BuildTime=${BUILD_TIME}"

          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "${BUILD_FLAGS}" -o dist/nyxproxy-linux-amd64 ./cmd/proxy
          CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "${BUILD_FLAGS}" -o dist/nyxproxy-linux-arm64 ./cmd/proxy

          cp config.example.yaml dist/
          cp scripts/quick-setup.sh dist/

          (cd dist && sha256sum * > sha256sums.txt)

      - name: Create Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/nyxproxy-linux-amd64
            dist/nyxproxy-linux-arm64
            dist/config.example.yaml
            dist/quick-setup.sh
            dist/sha256sums.txt
          generate_release_notes: true
```

- [ ] **Step 3: Update the Docker workflow paths and cache**

In `.github/workflows/docker.yml`, add these paths under `pull_request.paths`:

```yaml
      - "web/**"
      - "scripts/build-web.sh"
```

In the `Build and push` step, add:

```yaml
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 4: Update the English README**

Apply these edits to `README.md`:

1. Change the Go badge to:

```markdown
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
```

2. Add to the table of contents:

```markdown
- [Web Console](#-web-console)
```

3. Add a feature block after the Container Ready section:

```markdown
### 🖥️ Built-in Web Console
- **Browser-based management** - dashboard, configuration editor, and live logs
- **Transactional apply** - invalid changes are rejected; failed applies roll back to the last good config
- **Single binary and single container** - the Next.js UI is embedded; no Node runtime in production
- **Secure by default** - password login, CSRF protection, login rate limiting, optional TLS
```

4. In the Docker quick start, after `docker compose logs -f nyxproxy`, add:

```markdown
Open `http://YOUR_SERVER:18081` and sign in with `WEB_USER` and `WEB_PASS`
from `.env.docker`.
```

5. Replace every occurrence of:

```markdown
# edit .env.docker: at least set IFACE and PROXY_PASS
```

with:

```markdown
# edit .env.docker: set IFACE, PROXY_PASS, and WEB_PASS
```

6. In the quick-setup prompt list, after the IP age limit line, add:

```markdown
- Enable web console? (y/N)
- Web console username, password, and port (default: 18081)
```

7. Delete the Windows download note:

```markdown
Windows builds are published as `nyxproxy-windows-amd64.exe` and
`nyxproxy-windows-arm64.exe` in the release assets.
```

8. In the manual Docker run example, add the persistent volume and console variables:

```bash
docker run -d \
  --name nyxproxy \
  --network host \
  --cap-drop ALL --cap-add NET_ADMIN \
  --security-opt no-new-privileges:true \
  --read-only --tmpfs /run \
  -v nyxproxy-data:/data \
  -e IFACE=eth0 \
  -e LISTEN_PORT=18080 \
  -e POOL_SIZE=64 \
  -e PROXY_USER=admin \
  -e PROXY_PASS=change_me \
  -e WEB_ENABLED=true \
  -e WEB_PASS=change_me_too \
  nyxproxy-oss
```

9. Add these rows to the Docker environment table:

```markdown
| `WEB_ENABLED` | `true` | Enable the web console |
| `WEB_LISTEN` | `0.0.0.0` | Console listen address (host networking) |
| `WEB_PORT` | `18081` | Console port |
| `WEB_USER` | `admin` | Console username |
| `WEB_PASS` | *(required when enabled)* | Console password; only used when seeding a new volume |
| `WEB_TLS_CERT_FILE` | empty | Container path to a TLS certificate (read-only bind mount) |
| `WEB_TLS_KEY_FILE` | empty | Container path to the TLS private key |
```

10. Add a note after the table:

```markdown
The first start seeds `/data/config.yaml` from these environment variables. After
that, the file is authoritative: changing `.env.docker` no longer changes the
running configuration. Use the web console to edit settings, or delete the
`nyxproxy-data` volume to seed again. `PREFIX` and `IFACE` continue to be applied
at runtime through `NYXPROXY_PREFIX` and `NYXPROXY_INTERFACE`.
```

11. Add a new section before `## 💻 Usage Examples`:

```markdown
## 🖥️ Web Console

The console is served by the same binary and container as the proxy. It provides:

- a live dashboard (proxy state, address pool, traffic counters, last apply result)
- a full configuration editor with server-side validation
- automatic apply with rollback when a component cannot start
- a live log stream over Server-Sent Events

### Docker

Set `WEB_PASS` in `.env.docker`, start the stack, and open
`http://YOUR_SERVER:18081`. With host networking the port is exposed directly on
the host; restrict it with a firewall or a reverse proxy.

For built-in TLS, mount certificates read-only and set:

```yaml
web:
  tls_cert_file: /certs/tls.crt
  tls_key_file: /certs/tls.key
```

```bash
docker run ... -v "$PWD/certs:/certs:ro" ...
```

### Bare metal

Enable the console in `config.yaml`:

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

The bare-metal default binds to loopback. Use an SSH tunnel
(`ssh -L 18081:127.0.0.1:18081 user@server`) or a reverse proxy. If you put the
console behind nginx or Caddy, preserve the `Host` header, disable proxy
buffering for `/api/logs/stream`, and only set `trust_proxy_headers: true` when
the console is not directly exposed.

Saving a configuration restarts the proxy (and monitoring, if its settings
changed). Existing proxy connections are drained for up to 5 seconds. Invalid
configurations are rejected before anything is stopped. If the new configuration
cannot start, the old one is restored and the file on disk is left unchanged.
```

12. In the Requirements section, delete:

```markdown
- **Windows binaries**: published for amd64/arm64, but IPv6 address rotation requires Linux (`iproute2` + `NET_ADMIN`); Windows builds are suitable for non-rotating proxy use and testing
```

13. In Requirements, add:

```markdown
- **Go**: 1.27+ when building from source
- **Node**: 24 LTS and pnpm 12.9.1 when building the frontend
```

14. In Advanced Usage, replace:

```markdown
NyxProxy currently always reads `config.yaml` from the current working directory.
```

with:

```markdown
By default NyxProxy reads `./config.yaml`. Use `-config` or `NYXPROXY_CONFIG`
to point at a different file.
```

Then update the multiple-instance example to use `-config`:

```bash
(cd proxy-8080 && sudo ./nyxproxy -config /root/proxy-8080/config.yaml) &
(cd proxy-8081 && sudo ./nyxproxy -config /root/proxy-8081/config.yaml) &
```

Add the sentence:

```markdown
An explicit `-config` path is independent of the working directory;
`NYXPROXY_CONFIG` provides the same override through the environment.
```

15. In Configuration, add the console block:

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"   # use 0.0.0.0 only behind a firewall/proxy
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

16. In Running as Daemon (Systemd), add this before the service file:

```bash
sudo mkdir -p /etc/nyxproxy
sudo chmod 700 /etc/nyxproxy
sudo cp config.yaml /etc/nyxproxy/config.yaml
sudo chmod 600 /etc/nyxproxy/config.yaml
```

and change the service's working directory and command to:

```ini
WorkingDirectory=/etc/nyxproxy
ExecStart=/root/nyxproxy -config /etc/nyxproxy/config.yaml
```

- [ ] **Step 5: Update the Chinese README**

Apply the same changes to `README-ZH.md` with these translations:

1. Go badge:

```markdown
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
```

2. Table of contents:

```markdown
- [Web 管理面板](#-web-管理面板)
```

3. Feature block:

```markdown
### 🖥️ 内置 Web 管理面板
- **浏览器管理** —— 仪表盘、配置编辑器和实时日志
- **事务性应用** —— 非法配置在停止任何组件前被拒绝；启动失败会自动回滚
- **单二进制、单容器** —— Next.js 界面已嵌入，生产环境不依赖 Node
- **默认安全** —— 密码登录、CSRF 防护、登录限流、可选内置 TLS
```

4. Docker quick start addition:

```markdown
打开 `http://你的服务器:18081`，使用 `.env.docker` 中的 `WEB_USER` 和
`WEB_PASS` 登录。
```

5. Replace every occurrence of:

```markdown
# 编辑 .env.docker：至少设置 IFACE 和 PROXY_PASS
```

with:

```markdown
# 编辑 .env.docker：设置 IFACE、PROXY_PASS 和 WEB_PASS
```

6. In the quick-setup prompt list, after the IP age limit line, add:

```markdown
- 是否启用 Web 面板？(y/N)
- Web 面板用户名、密码和端口（默认：18081）
```

7. Delete the Windows note:

```markdown
Windows 版本以 `nyxproxy-windows-amd64.exe` 和
`nyxproxy-windows-arm64.exe` 的形式发布在 release 资源中。
```

8. Add the same volume and web environment variables to the manual `docker run`
example as the English README.

9. Add the `WEB_*` environment table, translated:

```markdown
| `WEB_ENABLED` | `true` | 是否启用 Web 管理面板 |
| `WEB_LISTEN` | `0.0.0.0` | 面板监听地址（host 网络） |
| `WEB_PORT` | `18081` | 面板端口 |
| `WEB_USER` | `admin` | 面板用户名 |
| `WEB_PASS` | 启用时必填 | 面板密码；仅在首次生成配置时使用 |
| `WEB_TLS_CERT_FILE` | 空 | 容器内 TLS 证书路径（只读挂载） |
| `WEB_TLS_KEY_FILE` | 空 | 容器内 TLS 私钥路径 |
```

10. Add the volume note:

```markdown
首次启动会用环境变量生成 `/data/config.yaml`。之后该文件是唯一权威来源：
修改 `.env.docker` 不会改变已有配置。请用 Web 面板修改配置，或删除
`nyxproxy-data` 卷后重新生成。`PREFIX` 和 `IFACE` 仍会通过
`NYXPROXY_PREFIX` / `NYXPROXY_INTERFACE` 在每次启动时生效。
```

11. Add the Web Console section before `## 💻 使用示例`:

```markdown
## 🖥️ Web 管理面板

管理面板与代理运行在同一个二进制和容器中，提供：

- 实时仪表盘（代理状态、地址池、流量计数、上次应用结果）
- 完整的配置编辑器和服务端校验
- 自动应用；组件启动失败时自动回滚
- 基于 SSE 的实时日志

### Docker

在 `.env.docker` 中设置 `WEB_PASS`，启动后打开
`http://你的服务器:18081`。host 网络下端口直接暴露在宿主机；请用防火墙或
反向代理限制访问。

内置 TLS 时，把证书只读挂载进容器，并设置：

```yaml
web:
  tls_cert_file: /certs/tls.crt
  tls_key_file: /certs/tls.key
```

```bash
docker run ... -v "$PWD/certs:/certs:ro" ...
```

### 裸机

在 `config.yaml` 中启用：

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

裸机默认只监听回环地址。使用 SSH 隧道
（`ssh -L 18081:127.0.0.1:18081 user@server`）或反向代理。若使用 nginx /
Caddy，请保留 `Host` 头，对 `/api/logs/stream` 关闭代理缓冲，并且只在面板
不直接暴露时设置 `trust_proxy_headers: true`。

保存配置会重启代理（监控配置变化时也会重启监控），现有连接最多等待 5 秒
完成 drain。非法配置会在停止任何组件之前被拒绝；新配置启动失败时会恢复旧
配置，磁盘文件保持不变。
```

12. Requirements: delete the Windows line and add Go/Node requirements in Chinese:

```markdown
- **Go**：从源码构建时需要 1.27+
- **Node**：构建前端时需要 Node 24 LTS 和 pnpm 12.9.1
```

13. In Advanced Usage, replace:

```markdown
NyxProxy 目前始终从当前工作目录读取 `config.yaml`。
```

with:

```markdown
默认读取 `./config.yaml`；使用 `-config` 或 `NYXPROXY_CONFIG` 可以指定其他文件。
```

Then use `-config` in the multiple-instance commands exactly as the English
README, and add:

```markdown
显式指定 `-config` 后，配置路径与工作目录无关；也可以通过
`NYXPROXY_CONFIG` 环境变量覆盖。
```

14. Add this block to the Chinese Configuration section:

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"   # 仅在防火墙/反向代理之后使用 0.0.0.0
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

15. In 以守护进程运行（Systemd）, add before the service file:

```bash
sudo mkdir -p /etc/nyxproxy
sudo chmod 700 /etc/nyxproxy
sudo cp config.yaml /etc/nyxproxy/config.yaml
sudo chmod 600 /etc/nyxproxy/config.yaml
```

and change the service to:

```ini
WorkingDirectory=/etc/nyxproxy
ExecStart=/root/nyxproxy -config /etc/nyxproxy/config.yaml
```

- [ ] **Step 6: Update `CONTRIBUTING.md`**

Replace the Prerequisites block:

```markdown
### Prerequisites

- Go 1.27 or later
- Linux (the proxy manages Linux network interfaces; Windows is not supported)
- Node 24 LTS and pnpm 12.9.1 for the web console
- Git
```

Add the frontend commands to the Build and Run section:

```bash
# Build the embedded web console
pnpm --dir web install --frozen-lockfile
pnpm --dir web build

# Build the Go binary
go build -o nyxproxy ./cmd/proxy
```

- [ ] **Step 7: Update `config.example.yaml`**

Append:

```yaml
web:
  # Disabled by default so existing deployments keep their current behavior.
  # Set enabled: true and a non-empty password to use the web console.
  enabled: false
  listen_address: "127.0.0.1"
  port: 18081
  username: admin
  password: change_me_too
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

- [ ] **Step 8: Update `docs/CONFIG.md`**

1. Replace the Configuration File section's first paragraph with:

```markdown
NyxProxy reads its configuration from the first path found in this order:

1. `-config <path>`
2. `NYXPROXY_CONFIG`
3. `./config.yaml`

The web console and the `Store` write back to the resolved path. Docker uses
`/data/config.yaml`.
```

2. Add `web` to the configuration structure block:

```yaml
web:
  enabled: boolean
  listen_address: string
  port: integer
  username: string
  password: string
  tls_cert_file: string
  tls_key_file: string
  trust_proxy_headers: boolean
  cookie_secure: boolean
```

3. Append a full Web Console section:

```markdown
## Web Console Settings

### `web.enabled`

- **Type**: `boolean`
- **Default**: `false`
- **Restart required**: Yes

Enables the browser console. Old configurations without a `web` section keep the
console disabled after upgrading.

### `web.listen_address`

- **Type**: `string`
- **Default**: `127.0.0.1`
- **Restart required**: Yes

IP address or `localhost`. The Docker template uses `0.0.0.0`; bare metal should
stay on loopback and use an SSH tunnel or reverse proxy.

### `web.port`

- **Type**: `integer`
- **Default**: `18081`
- **Range**: 1-65535
- **Restart required**: Yes

Must not conflict with the proxy or monitoring listener on the same address.

### `web.username`

- **Type**: `string`
- **Default**: `admin`
- **Required when enabled**: Yes

Changing this field through the console invalidates all sessions and requires a
new login.

### `web.password`

- **Type**: `string`
- **Required when enabled**: Yes

Stored in plain text in the 0600 configuration file. `GET /api/config` never
returns it. Submitting an empty password through the console keeps the current
password.

### `web.tls_cert_file` / `web.tls_key_file`

- **Type**: `string`
- **Default**: empty
- **Restart required**: Yes

Serve the console over TLS. Both must be set together and must be readable.
Minimum TLS version is 1.2. The console uses HTTP when both are empty.

### `web.trust_proxy_headers`

- **Type**: `boolean`
- **Default**: `false`
- **Restart required**: Yes

Trust `X-Forwarded-For` and `X-Forwarded-Proto` from a reverse proxy. Enable only
when the console is not directly reachable. The reverse proxy must preserve the
`Host` header or Origin checks will fail.

### `web.cookie_secure`

- **Type**: `boolean`
- **Default**: `false`
- **Restart required**: Yes

Force the `Secure` flag on the session cookie when TLS terminates at a reverse
proxy and forwarded-proto headers are not used. With built-in TLS the flag is set
automatically.

## Environment overrides

- `NYXPROXY_CONFIG`: configuration path when `-config` is not set.
- `NYXPROXY_PREFIX`: overrides `network.ipv6_subnet` in memory only. The console
  shows the field as environment-managed and rejects edits.
- `NYXPROXY_INTERFACE`: overrides `network.interface_name` in memory only.

Runtime overrides are never written back to the configuration file.
```

- [ ] **Step 9: Update `scripts/quick-setup.sh`**

After the rotation prompts (`Max age in minutes before rotation`), insert:

```bash
# Optional web console
echo
echo -n "  Enable web console? (y/N) "
read -r WEB_ENABLED_CHOICE
WEB_ENABLED=false
WEB_USER=admin
WEB_PASS=""
WEB_PORT=18081

if [[ $WEB_ENABLED_CHOICE =~ ^[Yy]$ ]]; then
    WEB_ENABLED=true
    echo -n "  Web username [admin]: "
    read WEB_USER
    WEB_USER=${WEB_USER:-admin}

    echo -n "  Web password: "
    read -s WEB_PASS
    echo
    while [ -z "$WEB_PASS" ]; do
        echo -e "${RED}  Password cannot be empty${NC}"
        echo -n "  Web password: "
        read -s WEB_PASS
        echo
    done

    echo -n "  Web port [18081]: "
    read WEB_PORT
    WEB_PORT=${WEB_PORT:-18081}
fi
```

Append this block inside the generated config heredoc, after the `logging:`
section:

```yaml
web:
  enabled: $WEB_ENABLED
  listen_address: "127.0.0.1"
  port: $WEB_PORT
  username: "$WEB_USER"
  password: "$WEB_PASS"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

After the setup summary, add:

```bash
if [ "$WEB_ENABLED" = "true" ]; then
    echo -e "${GREEN}Web Console:${NC}"
    echo "  Local URL:    http://127.0.0.1:$WEB_PORT"
    echo "  Remote:       ssh -L $WEB_PORT:127.0.0.1:$WEB_PORT root@YOUR_SERVER"
    echo "  Username:     $WEB_USER"
    echo
fi
```

- [ ] **Step 10: Remove all remaining Windows references**

Run:

```bash
grep -rni "windows\|\.exe" --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=superpowers .
```

Expected: no matches in README, README-ZH, CONTRIBUTING, workflows, Docker files,
or user docs. Generic `*.exe` ignore lines in `.gitignore`/`.dockerignore` and
`runtime.GOOS != "windows"` guards in Go tests are allowed (the tests run on a
Windows development host, even though Windows is not a supported target).

- [ ] **Step 11: Run validation**

```bash
go test ./... -count=1
cd web && pnpm test && pnpm lint && pnpm build && cd ..
shellcheck docker/entrypoint.sh scripts/build-web.sh
```

Expected: all pass. `pnpm lint` uses the Next.js ESLint configuration.

- [ ] **Step 12: Commit checkpoint**

```bash
git add .github README.md README-ZH.md CONTRIBUTING.md docs/CONFIG.md config.example.yaml scripts/quick-setup.sh
git commit -m "ci/docs: switch to pnpm builds and document the embedded web console"
```

Only run if the user has authorized commits.

## Task 16: End-to-End Verification and Acceptance

**Files:**
- Create: `scripts/smoke-test.sh`
- Create: `internal/config/example_test.go`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the built binary from Task 13, all API endpoints from Tasks 7-8, and the example config from Task 15
- Produces: a repeatable local/CI smoke test and a completed acceptance checklist

- [ ] **Step 1: Create the smoke test script**

Create `scripts/smoke-test.sh`:

```bash
#!/usr/bin/env bash
# Requires curl and python3. Runs a non-rotating IPv4 proxy so it works on a
# normal Linux host and on GitHub Actions; IPv6 address management is covered
# by the Go unit tests, not this smoke test.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${1:-$ROOT/dist/nyxproxy}"
WEB_PORT="${SMOKE_WEB_PORT:-18081}"
PROXY_PORT="${SMOKE_PROXY_PORT:-18080}"

if [ ! -x "$BIN" ]; then
  echo "binary not found or not executable: $BIN" >&2
  exit 1
fi

WORK="$(mktemp -d)"
PID=""
cleanup() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

cat >"$WORK/config.yaml" <<EOF
proxy:
  type: socks5
  listen_address: "127.0.0.1"
  listen_port: $PROXY_PORT
  username: "smoke"
  password: "smoke-pass"
network:
  interface_name: ""
  ipv4_enabled: true
  ipv6_enabled: false
  ipv6_subnet: ""
  dns_source_address: ""
  rotate_ipv6: false
  ipv6_pool_size: 1
  ipv6_max_usage: 1
  ipv6_max_age: 1
monitoring:
  enabled: false
  port: 19090
  allow_remote: false
logging:
  debug_level: 0
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: $WEB_PORT
  username: "admin"
  password: "smoke-secret"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
EOF

"$BIN" -config "$WORK/config.yaml" >"$WORK/stdout.log" 2>"$WORK/stderr.log" &
PID=$!

ready=false
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$WEB_PORT/healthz" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "server exited before becoming ready" >&2
    cat "$WORK/stderr.log" >&2
    exit 1
  fi
  sleep 0.1
done
if [ "$ready" != "true" ]; then
  echo "server did not become ready" >&2
  cat "$WORK/stderr.log" >&2
  exit 1
fi

curl -fsS -c "$WORK/cookies.txt" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"smoke-secret"}' \
  "http://127.0.0.1:$WEB_PORT/api/auth/login" >"$WORK/login.json"

CSRF="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["csrf_token"])' "$WORK/login.json")"

curl -fsS -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/status" >"$WORK/status.json"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["proxy"]["state"] == "running", d' "$WORK/status.json"

curl -fsS -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/config" >"$WORK/config.json"

python3 - "$WORK/config.json" "$WORK/update.json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1]))["config"]
payload["logging"]["debug_level"] = 1
json.dump(payload, open(sys.argv[2], "w"))
PY

curl -fsS -b "$WORK/cookies.txt" \
  -X PUT \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  --data-binary @"$WORK/update.json" \
  "http://127.0.0.1:$WEB_PORT/api/config" >"$WORK/apply.json"

python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["changed"] is True, d' "$WORK/apply.json"

if [ "$(stat -c %a "$WORK/config.yaml")" != "600" ]; then
  echo "config file mode is not 0600" >&2
  exit 1
fi

curl -sS -N --max-time 1 -D "$WORK/sse.headers" -o /dev/null \
  -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/logs/stream" || true
grep -qi '^content-type: text/event-stream' "$WORK/sse.headers"

curl -fsS --socks5 "127.0.0.1:$PROXY_PORT" --proxy-user "smoke:smoke-pass" \
  "http://127.0.0.1:$WEB_PORT/healthz" >/dev/null

kill "$PID"
wait "$PID"
PID=""

echo "smoke test passed"
```

- [ ] **Step 2: Add the example config parse test**

Create `internal/config/example_test.go`:

```go
package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExampleConfigParses(t *testing.T) {
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatalf("read config.example.yaml: %v", err)
	}

	var cfg ProxyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.example.yaml is not valid YAML: %v", err)
	}
	if cfg.Proxy.Type == "" {
		t.Fatal("proxy.type did not bind from config.example.yaml")
	}
	if cfg.Web.Port != 18081 {
		t.Fatalf("web.port = %d, want 18081", cfg.Web.Port)
	}
}
```

- [ ] **Step 3: Wire the smoke test into CI**

In `.github/workflows/ci.yml`, change the shellcheck step to:

```yaml
      - name: Shellcheck entrypoint and build scripts
        run: shellcheck docker/entrypoint.sh scripts/build-web.sh scripts/smoke-test.sh
```

After the `Build Linux binaries` step in the `go` job, add:

```yaml
      - name: Smoke test binary
        run: bash scripts/smoke-test.sh dist/nyxproxy-linux-amd64

      - name: Validate compose file
        run: |
          cp .env.docker.example .env.docker
          docker compose config -q
```

The `go` job already downloads `web-dist` into `internal/webui/dist`, so the
binary under test contains the real frontend.

- [ ] **Step 4: Run the full Go suite**

```bash
gofmt -l .
go vet ./...
go test -race ./... -count=1
```

Expected: `gofmt -l .` prints nothing; vet and tests pass. If `gofmt -l` lists
files, run `gofmt -w <files>` and re-run.

- [ ] **Step 5: Run the full frontend suite**

```bash
cd web
pnpm install --frozen-lockfile
pnpm lint
pnpm test
pnpm build
cd ..
```

Expected: all commands exit 0. `pnpm lint` must not prompt for ESLint setup;
`web/.eslintrc.json` from Task 10 is present.

- [ ] **Step 6: Build the embedded binary**

```bash
bash scripts/build-web.sh
test -f internal/webui/dist/index.html
CGO_ENABLED=0 go build -trimpath -o dist/nyxproxy ./cmd/proxy
```

Expected: `internal/webui/dist/index.html` exists and the binary builds.

- [ ] **Step 7: Run the smoke test**

On Linux or in CI:

```bash
bash scripts/smoke-test.sh ./dist/nyxproxy
```

Expected:

```text
smoke test passed
```

On the current Windows host, the Linux binary cannot execute; run the unit,
race, vet, frontend, and build checks locally and rely on the CI smoke step.
The smoke script deliberately uses `rotate_ipv6: false`, so it does not need
`NET_ADMIN` or a real IPv6 `/64`.

- [ ] **Step 8: Validate deployment files**

```bash
cp .env.docker.example .env.docker
docker compose config -q
rm .env.docker
grep -Rni "windows\|\.exe" --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=superpowers .
grep -RnE '(^|[^[:alnum:]])npm([^[:alnum:]]|$)' --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=superpowers --exclude=pnpm-lock.yaml . || true
```

Expected:

- `docker compose config -q` exits 0 when Docker is installed. If Docker is
  unavailable, skip and note that CI validates it.
- The Windows search returns no README/workflow/user-doc matches. The generic
  `*.exe` lines in `.gitignore` and `.dockerignore`, the `runtime.GOOS !=
  "windows"` test guards, and plan/spec files under `docs/superpowers/` are
  allowed (`docs/superpowers/` is excluded because it documents the removal).
- The `npm` search returns nothing; `pnpm` is the only package manager.

- [ ] **Step 9: Complete the acceptance checklist**

Verify each item against the implementation and record the result:

- [ ] Every package-management command in CI, Docker, Makefile, scripts, and docs uses pnpm; `web/pnpm-lock.yaml` is committed; no `package-lock.json` or `yarn.lock` exists.
- [ ] `gofmt -l .` is empty, `go vet ./...` passes, `go test -race ./...` passes.
- [ ] `pnpm lint`, `pnpm test`, and `pnpm build` pass in `web/`.
- [ ] CI builds Linux amd64 and arm64 binaries; the Docker workflow builds both architectures.
- [ ] The final image contains no Node runtime; `node`/`pnpm` appear only in the Docker build stages.
- [ ] Login, session expiry, CSRF, Origin checks, rate limiting, config GET/PUT, apply/rollback, retry, interfaces, and SSE have automated tests.
- [ ] A config without a `web` section starts with the console disabled and behaves like the previous release.
- [ ] The smoke test covers `/healthz`, login, `/api/status`, config GET, config PUT with apply, file mode 0600, SSE headers, and a SOCKS5 request.
- [ ] `README.md` and `README-ZH.md` contain matching Web Console, Docker volume, port, environment, and security sections; Windows content is removed from both.
- [ ] `config.example.yaml`, `docs/CONFIG.md`, and `scripts/quick-setup.sh` document and generate the `web` section.

- [ ] **Step 10: Final commit checkpoint**

```bash
git add scripts/smoke-test.sh internal/config/example_test.go .github/workflows/ci.yml
git commit -m "test: add end-to-end smoke coverage and example config validation"
```

Only run if the user has authorized commits.
