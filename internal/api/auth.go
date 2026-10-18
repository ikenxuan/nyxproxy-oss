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

	cleanupMu      sync.Mutex
	cleanupStop    chan struct{}
	cleanupStarted bool
	cleanupStopped bool
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
		cleanupStop:       make(chan struct{}),
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
	if err != nil || u.Host == "" {
		return ErrOriginMismatch
	}
	host := r.Host
	if a.trustProxyHeaders {
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			// A trusted proxy (including the Next.js dev server rewrite)
			// changes Host before forwarding. The first non-empty value is
			// the host the client used for the request.
			for _, candidate := range strings.Split(xfh, ",") {
				if candidate = strings.TrimSpace(candidate); candidate != "" {
					host = candidate
					break
				}
			}
		}
	}
	if !strings.EqualFold(u.Host, host) {
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
	a.cleanupMu.Lock()
	defer a.cleanupMu.Unlock()
	if a.cleanupStarted || a.cleanupStopped {
		return
	}
	a.cleanupStarted = true
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.mu.Lock()
				a.cleanupLocked(a.now())
				a.mu.Unlock()
			case <-a.cleanupStop:
				return
			}
		}
	}()
}

// StopCleanup stops the background session sweeper. It is idempotent and safe
// to call before StartCleanup.
func (a *Auth) StopCleanup() {
	a.cleanupMu.Lock()
	defer a.cleanupMu.Unlock()
	if a.cleanupStopped {
		return
	}
	a.cleanupStopped = true
	close(a.cleanupStop)
}

func (a *Auth) ClientIP(r *http.Request) string {
	if a.trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// The rightmost non-empty hop is the address appended by the
			// directly connected trusted proxy. Leftmost hops are
			// client-controlled and must not key rate limiting.
			hops := strings.Split(xff, ",")
			for i := len(hops) - 1; i >= 0; i-- {
				if hop := strings.TrimSpace(hops[i]); hop != "" {
					return hop
				}
			}
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
