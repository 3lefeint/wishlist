package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPA(t *testing.T) {
	files := fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/a.js":   {Data: []byte("js")},
		"manifest.json": {Data: []byte("{}")},
	}
	h := spa(files)
	tests := []struct {
		path      string
		code      int
		body      string
		cacheWant string
	}{
		{"/", 200, "<html>app</html>", "no-cache"},
		{"/circles/5", 200, "<html>app</html>", "no-cache"},
		{"/assets/a.js", 200, "js", "public, max-age=31536000, immutable"},
		{"/manifest.json", 200, "{}", "no-cache"},
		{"/api/unknown", 404, "", ""},
		{"/api", 404, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.code {
				t.Fatalf("code = %d, want %d", rec.Code, tc.code)
			}
			if tc.code == 200 && rec.Body.String() != tc.body {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.body)
			}
			if got := rec.Header().Get("Cache-Control"); tc.cacheWant != "" && got != tc.cacheWant {
				t.Fatalf("cache = %q, want %q", got, tc.cacheWant)
			}
		})
	}
}
