package main

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"
)

// precompressed is in order of preference. `vite build` writes these files.
var precompressed = []struct{ encoding, suffix string }{
	{"br", ".br"},
	{"gzip", ".gz"},
}

// staticHandler serves the output of `vite build`. Files in assets/ have a hash
// in their name, so they never change. Anything else must be checked every time.
func staticHandler(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		w.Header().Set("Vary", "Accept-Encoding")
		if _, err := fs.Stat(files, name); err == nil {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			if servePrecompressed(w, r, files, name) {
				return
			}
		} else if name == "" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func servePrecompressed(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	for _, candidate := range precompressed {
		if !acceptsEncoding(r, candidate.encoding) {
			continue
		}
		file, err := files.Open(name + candidate.suffix)
		if err != nil {
			continue
		}
		defer file.Close()
		info, err := file.Stat()
		seeker, ok := file.(io.ReadSeeker)
		if err != nil || !ok {
			continue
		}
		w.Header().Set("Content-Encoding", candidate.encoding)
		// The original name, so the Content-Type is that of the uncompressed file.
		http.ServeContent(w, r, name, info.ModTime(), seeker)
		return true
	}
	return false
}

func acceptsEncoding(r *http.Request, encoding string) bool {
	return slices.ContainsFunc(strings.Split(r.Header.Get("Accept-Encoding"), ","), func(item string) bool {
		token, params, _ := strings.Cut(strings.TrimSpace(item), ";")
		return token == encoding && strings.TrimSpace(params) != "q=0"
	})
}
