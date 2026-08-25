package webdist

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerFallsBackOnlyForWebRoutes(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("web app")},
		"assets/app.js": &fstest.MapFile{Data: []byte("javascript")},
	}
	handler := NewHandler(fs.FS(assets))

	for _, path := range []string{"/", "/projects/bee/overview"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != "web app" {
			t.Fatalf("%s: status = %d, body = %q", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/api/missing", "/health/missing"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, response.Code)
		}
	}
}
