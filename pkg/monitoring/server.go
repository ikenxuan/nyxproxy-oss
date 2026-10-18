package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/metrics"
)

// Server represents the monitoring HTTP server
type Server struct {
	config   *config.ProxyConfig
	metrics  *metrics.Metrics
	server   *http.Server
	version  string
	mu       sync.Mutex
	listener net.Listener
}

// New creates a new monitoring server
func New(cfg *config.ProxyConfig, m *metrics.Metrics, version string) *Server {
	return &Server{
		config:  cfg,
		metrics: m,
		version: version,
	}
}

// Start binds the monitoring listener and returns; serving continues in the
// background until Stop is called.
func (s *Server) Start() error {
	if !s.config.Monitoring.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/config", s.handleConfig)

	// Determine listen address
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

// Addr returns the bound monitoring address, or an empty string when stopped.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop shuts down the monitoring server. It is safe to call more than once.
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

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := s.metrics.GetStats()

	response := map[string]interface{}{
		"status":  "ok",
		"uptime":  stats.Uptime,
		"version": s.version,
	}

	json.NewEncoder(w).Encode(response)
}

// handleStats handles statistics requests
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := s.metrics.GetStats()
	json.NewEncoder(w).Encode(stats)
}

// handleConfig handles configuration requests (sanitized)
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"proxy": map[string]interface{}{
			"type":           s.config.GetProxyType(),
			"listen_address": s.config.GetListenAddress(),
			"listen_port":    s.config.GetListenPort(),
			"username":       s.config.GetUsername(),
		},
		"network": map[string]interface{}{
			"interface_name": s.config.Network.InterfaceName,
			"ipv4_enabled":   s.config.Network.IPv4Enabled,
			"ipv6_enabled":   s.config.Network.IPv6Enabled,
		},
		"monitoring": map[string]interface{}{
			"enabled":      s.config.Monitoring.Enabled,
			"port":         s.config.Monitoring.Port,
			"allow_remote": s.config.Monitoring.AllowRemote,
		},
		"logging": map[string]interface{}{
			"debug_level": s.config.GetDebugLevel(),
		},
	}

	json.NewEncoder(w).Encode(response)
}
