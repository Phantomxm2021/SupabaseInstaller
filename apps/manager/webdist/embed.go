package webdist

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// Assets contains the production React build copied here by the Manager image build.
//
//go:embed all:dist
var embedded embed.FS

func Embedded() fs.FS {
	assets, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}

func NewHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	serveIndex := func(response http.ResponseWriter, request *http.Request) {
		contents, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.NotFound(response, request)
			return
		}
		http.ServeContent(response, request, "index.html", time.Time{}, strings.NewReader(string(contents)))
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		cleaned := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if cleaned == "." {
			serveIndex(response, request)
			return
		}
		if _, err := fs.Stat(assets, cleaned); err == nil {
			files.ServeHTTP(response, request)
			return
		}
		if strings.HasPrefix(request.URL.Path, "/api/") || request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/health/") || request.URL.Path == "/health" {
			http.NotFound(response, request)
			return
		}
		serveIndex(response, request)
	})
}
