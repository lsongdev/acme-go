// Package web serves the management UI: Preact and htm ES modules loaded
// directly by the browser, with no build step.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Handler serves the UI. It expects the JSON API at ./api/ relative to the
// page.
func Handler() http.Handler {
	root, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err) // the embedded directory is checked at build time
	}
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Modules are not fingerprinted; always revalidate so a new binary's
		// UI is picked up immediately.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}
