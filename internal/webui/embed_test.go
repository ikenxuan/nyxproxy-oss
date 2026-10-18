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
		"index.html":        {Data: []byte("root")},
		"config/index.html": {Data: []byte("config-page")},
		"_next/static/a.js": {Data: []byte("console.log(1)")},
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
