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

type serverState int

const (
	stateIdle serverState = iota
	stateStarting
	stateRunning
	stateFailed
	stateStopped
)

type Server struct {
	opts Options
	auth *Auth

	httpServer *http.Server
	mu         sync.Mutex
	state      serverState
	listener   net.Listener
	serveErr   error
	done       chan struct{}
	doneOnce   sync.Once

	stopStreams     chan struct{}
	stopStreamsOnce sync.Once
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
	s := &Server{opts: opts, auth: opts.Auth, done: make(chan struct{}), stopStreams: make(chan struct{})}
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

func (s *Server) Start() error {
	s.mu.Lock()
	if s.state != stateIdle {
		err := s.startStateErrorLocked()
		s.mu.Unlock()
		return err
	}
	s.state = stateStarting
	s.mu.Unlock()

	addr := net.JoinHostPort(s.opts.Config.Web.ListenAddress, strconv.Itoa(s.opts.Config.Web.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return s.failStart(fmt.Errorf("web console listen: %w", err))
	}

	if s.opts.Config.Web.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(s.opts.Config.Web.TLSCertFile, s.opts.Config.Web.TLSKeyFile)
		if err != nil {
			ln.Close()
			return s.failStart(fmt.Errorf("load tls certificate: %w", err))
		}
		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
	}

	s.mu.Lock()
	if s.state != stateStarting {
		s.mu.Unlock()
		ln.Close()
		return fmt.Errorf("web console startup aborted: server already stopped")
	}
	s.listener = ln
	s.state = stateRunning
	s.auth.StartCleanup()
	go func() {
		err := s.httpServer.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		s.mu.Lock()
		s.serveErr = err
		s.mu.Unlock()
		s.closeDone()
	}()
	s.mu.Unlock()

	log.Printf("web console listening on %s (tls=%v)", ln.Addr().String(), s.opts.Config.Web.TLSCertFile != "")
	return nil
}

func (s *Server) startStateErrorLocked() error {
	switch s.state {
	case stateFailed:
		return fmt.Errorf("web console server failed to start")
	case stateStopped:
		return fmt.Errorf("web console server is stopped")
	default:
		return fmt.Errorf("web console server is already started")
	}
}

// failStart records a start failure, transitions to the failed state, and
// closes the done channel so Wait reports the error without blocking.
func (s *Server) failStart(err error) error {
	s.mu.Lock()
	if s.state == stateStarting {
		s.state = stateFailed
		s.serveErr = err
	}
	s.mu.Unlock()
	s.closeDone()
	return err
}

func (s *Server) closeDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

func (s *Server) Wait() error {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serveErr
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Unblock any open log streams so graceful shutdown does not wait on them.
	s.closeStopStreams()

	s.mu.Lock()
	switch s.state {
	case stateIdle, stateStarting:
		s.state = stateStopped
		s.mu.Unlock()
		s.auth.StopCleanup()
		s.closeDone()
		return nil
	case stateRunning:
		s.state = stateStopped
		ln := s.listener
		s.mu.Unlock()

		s.auth.StopCleanup()
		// Close the captured listener before Shutdown so the accept loop
		// exits promptly, then drain live connections gracefully.
		if ln != nil {
			_ = ln.Close()
		}
		err := s.httpServer.Shutdown(ctx)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		s.closeDone()
		return err
	default: // stateFailed, stateStopped
		s.mu.Unlock()
		s.auth.StopCleanup()
		s.closeDone()
		return nil
	}
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// closeStopStreams signals every open log stream to return. It is safe to
// call multiple times and from any state.
func (s *Server) closeStopStreams() {
	s.stopStreamsOnce.Do(func() { close(s.stopStreams) })
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
