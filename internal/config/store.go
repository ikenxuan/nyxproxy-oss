package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// Store owns the in-memory configuration and persists updates atomically.
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

// Prepare writes cfg to a temporary file next to the store path and returns
// its name. The caller must call Commit or Discard with the returned path.
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

// Commit atomically replaces the store file with the prepared temp file and
// installs cfg as the new in-memory configuration.
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
