package api

import (
	"errors"
	"fmt"
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

func trustedAuth(t *testing.T) *Auth {
	t.Helper()
	return NewAuth("admin", "secret", AuthOptions{
		TrustProxyHeaders: true,
		Now:               time.Now,
		Sleep:             func(time.Duration) {},
	})
}

func TestOriginUsesForwardedHostWhenTrusted(t *testing.T) {
	a := trustedAuth(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18081/api/auth/login", nil)
	req.Host = "127.0.0.1:18081"
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("X-Forwarded-Host", "  localhost:3000  ")
	if err := a.CheckOrigin(req); err != nil {
		t.Fatalf("trusted forwarded host should match origin: %v", err)
	}
}

func TestOriginRejectsMismatchedForwardedHost(t *testing.T) {
	a := trustedAuth(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18081/api/auth/login", nil)
	req.Host = "127.0.0.1:18081"
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("X-Forwarded-Host", "localhost:3000")
	if !errors.Is(a.CheckOrigin(req), ErrOriginMismatch) {
		t.Fatal("expected origin mismatch against forwarded host")
	}
}

func TestOriginIgnoresSpoofedForwardedHostWhenUntrusted(t *testing.T) {
	a := newTestAuth(time.Now)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18081/api/auth/login", nil)
	req.Host = "127.0.0.1:18081"
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("X-Forwarded-Host", "localhost:3000")
	if !errors.Is(a.CheckOrigin(req), ErrOriginMismatch) {
		t.Fatal("untrusted forwarded host must not override the request host")
	}
}

func TestRateLimitUsesRightmostForwardedHop(t *testing.T) {
	a := NewAuth("admin", "secret", AuthOptions{
		TrustProxyHeaders: true,
		Now:               time.Now,
		Sleep:             func(time.Duration) {},
	})

	// Five failures whose leftmost (client-supplied) XFF element varies but
	// whose rightmost hop, appended by the connected proxy, stays constant.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d, 203.0.113.9", i+1))
		_, _ = a.Login(httptest.NewRecorder(), req, "admin", "wrong")
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.200, 203.0.113.9")
	_, err := a.Login(httptest.NewRecorder(), req, "admin", "secret")
	var rateErr *RateLimitedError
	if !errors.As(err, &rateErr) {
		t.Fatalf("error = %v, want RateLimitedError for constant rightmost hop", err)
	}

	// A different rightmost hop represents a different directly connected
	// proxy and must not inherit the other hop's failure window.
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.200, 203.0.113.10")
	if _, err := a.Login(httptest.NewRecorder(), req, "admin", "secret"); err != nil {
		t.Fatalf("login with different rightmost hop: %v", err)
	}
}

func TestStopCleanupIdempotent(t *testing.T) {
	a := newTestAuth(time.Now)
	a.StartCleanup()
	a.StopCleanup()
	a.StopCleanup()
	a.StartCleanup()
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
