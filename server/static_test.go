package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticHandler(t *testing.T) {
	files := fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html>")},
		"assets/a.wasm":      {Data: []byte("raw")},
		"assets/a.wasm.br":   {Data: []byte("brotli")},
		"assets/a.wasm.gz":   {Data: []byte("gzip")},
		"assets/plain-x.css": {Data: []byte("body{}")},
	}
	const immutable = "public, max-age=31536000, immutable"

	tests := []struct {
		name, path, acceptEncoding string
		status                     int
		body, encoding, cache      string
		contentType                string
	}{
		{name: "brotli preferred", path: "/assets/a.wasm", acceptEncoding: "gzip, br", status: 200, body: "brotli", encoding: "br", cache: immutable, contentType: "application/wasm"},
		{name: "gzip", path: "/assets/a.wasm", acceptEncoding: "gzip", status: 200, body: "gzip", encoding: "gzip", cache: immutable, contentType: "application/wasm"},
		{name: "refused encoding", path: "/assets/a.wasm", acceptEncoding: "br;q=0, gzip", status: 200, body: "gzip", encoding: "gzip", cache: immutable, contentType: "application/wasm"},
		{name: "no encoding", path: "/assets/a.wasm", status: 200, body: "raw", cache: immutable, contentType: "application/wasm"},
		{name: "asset without a compressed copy", path: "/assets/plain-x.css", acceptEncoding: "br", status: 200, body: "body{}", cache: immutable, contentType: "text/css; charset=utf-8"},
		{name: "page must be revalidated", path: "/", acceptEncoding: "br", status: 200, body: "<!doctype html>", cache: "no-cache", contentType: "text/html; charset=utf-8"},
		{name: "missing asset is not cached", path: "/assets/none.js", status: 404},
		{name: "outside the root", path: "/../secret", status: 404},
		{name: "no directory listing", path: "/assets/", status: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			r.URL.Path = tt.path
			if tt.acceptEncoding != "" {
				r.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			w := httptest.NewRecorder()
			staticHandler(files).ServeHTTP(w, r)

			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
			if got := w.Header().Get("Cache-Control"); got != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cache)
			}
			if tt.status != http.StatusOK {
				return
			}
			if got := w.Body.String(); got != tt.body {
				t.Errorf("body = %q, want %q", got, tt.body)
			}
			if got := w.Header().Get("Content-Encoding"); got != tt.encoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tt.encoding)
			}
			if got := w.Header().Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q", got)
			}
		})
	}
}
