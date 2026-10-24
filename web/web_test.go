package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The assets satisfy the panel module contract (version 1).
func TestManifestFollowsContract(t *testing.T) {
	data, err := fs.ReadFile(UI, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contract int      `json:"contract"`
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Entry    string   `json:"entry"`
		Styles   []string `json:"styles"`
		Home     string   `json:"home"`
		Nav      []struct {
			Path, Title string
		} `json:"nav"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Contract != 1 || !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(m.ID) || m.Title == "" {
		t.Fatalf("manifest = %+v", m)
	}
	for _, name := range append([]string{m.Entry, "index.html"}, m.Styles...) {
		if _, err := fs.Stat(UI, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	entry, _ := fs.ReadFile(UI, m.Entry)
	if !regexp.MustCompile(`export (function|const) mount\b`).Match(entry) {
		t.Error("entry does not export mount")
	}
	// Every navigation entry and the home page have a route.
	for _, path := range append([]string{m.Home}, func() (p []string) {
		for _, n := range m.Nav {
			p = append(p, n.Path)
		}
		return
	}()...) {
		if !strings.Contains(string(entry), `path: '`+path+`'`) {
			t.Errorf("no route for %s", path)
		}
	}
}

func TestStatic(t *testing.T) {
	h := Static(UI)
	get := func(path, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	index := get("/", "")
	if index.Code != 200 || !strings.Contains(index.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("index: %d %v", index.Code, index.Header())
	}
	if w := get("/", index.Header().Get("ETag")); w.Code != http.StatusNotModified {
		t.Fatalf("revalidation: %d", w.Code)
	}
	if w := get("/module.js", ""); w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("module.js: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	for _, path := range []string{"/missing.js", "/pages/", "/../web.go"} {
		if w := get(path, ""); w.Code != 404 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}
