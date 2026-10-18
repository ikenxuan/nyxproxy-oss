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
