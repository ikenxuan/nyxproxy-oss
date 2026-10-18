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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
