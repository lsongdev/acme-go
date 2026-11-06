package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// csp allows the panel's own scripts, styles, images and API calls, and the
// inline import map of index.html by its hash.
func csp(index []byte) string {
	scripts := "'self'"
	for _, m := range importMap.FindAllSubmatch(index, -1) {
		sum := sha256.Sum256(m[1])
		scripts += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return "script-src " + scripts + "; " + policy
}

// importMap finds inline import maps, which a script-src without a hash blocks.
var importMap = regexp.MustCompile(`(?s)<script type="importmap">(.*?)</script>`)

const policy = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// Standalone serves the module on its own: the UI at / and the API at
// /api/. A non-empty password requires HTTP basic authentication (any user
// name), which browsers also send on EventSource and WebSocket requests.
func (a *ACME) Standalone(password string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", a.Handler()))
	mux.Handle("/", Static(a.Assets()))
	var h http.Handler = http.NewCrossOriginProtection().Handler(mux)
	if password != "" {
		h = basicAuth(h, password)
	}
	return h
}

func basicAuth(next http.Handler, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte("admin")) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="acme", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Static serves an embedded file tree with content-hash ETags, and
// index.html at "/" with a strict Content-Security-Policy.
func Static(files fs.FS) http.Handler {
	etags := map[string]string{}
	fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, err := fs.ReadFile(files, p); err == nil {
				sum := sha256.Sum256(data)
				etags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
			}
		}
		return nil
	})
	index, _ := fs.ReadFile(files, "index.html")
	security := csp(index)
	server := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		etag, ok := etags[name]
		if !ok {
			http.NotFound(w, r) // no directory listings
			return
		}
		h := w.Header()
		h.Set("ETag", etag)
		// Embedded files change only with the binary: revalidate cheaply.
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Content-Type-Options", "nosniff")
		if name == "index.html" {
			h.Set("Content-Security-Policy", security)
			h.Set("Referrer-Policy", "same-origin")
			http.ServeFileFS(w, r, files, name) // FileServer would redirect index.html to ./
			return
		}
		server.ServeHTTP(w, r)
	})
}
