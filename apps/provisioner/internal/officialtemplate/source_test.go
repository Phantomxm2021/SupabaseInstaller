package officialtemplate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAllowsFiveMinutesForOfficialTemplateDownloads(t *testing.T) {
	source, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if source.client.Timeout < 5*time.Minute {
		t.Fatalf("official template timeout = %s, want at least 5m for a GitHub archive download", source.client.Timeout)
	}
}

func TestResolveDownloadsCachesAndReloadsOfficialRelease(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"supabase-self-hosted-v9.8.7/docker/docker-compose.yml":       "services:\n  db:\n    image: supabase/postgres:17.6.1.080\n",
		"supabase-self-hosted-v9.8.7/docker/.env.example":             "POSTGRES_PASSWORD=example\n",
		"supabase-self-hosted-v9.8.7/docker/volumes/db/_supabase.sql": "select 1;\n",
	})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/self-hosted/v9.8.7" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	source, err := New(t.TempDir(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	source.archiveBase = server.URL + "/"
	first, err := source.Resolve(context.Background(), "self-hosted/v9.8.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ref != "self-hosted/v9.8.7" || len(first.SHA256) != 64 || string(first.Compose()) == "" {
		t.Fatalf("unexpected snapshot: %#v", first)
	}
	second, err := source.Resolve(context.Background(), "self-hosted/v9.8.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || second.SHA256 != first.SHA256 {
		t.Fatalf("requests=%d second=%#v", requests, second)
	}
	if _, err := os.Stat(filepath.Join(source.cacheRoot, "self-hosted_v9.8.7", "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestResolveLatestUsesGreatestOfficialSelfHostedTag(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"root/docker/docker-compose.yml":       "services: {}\n",
		"root/docker/.env.example":             "A=B\n",
		"root/docker/volumes/db/_supabase.sql": "select 1;\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/refs/git/matching-refs/tags/self-hosted/":
			_, _ = w.Write([]byte(`[{"ref":"refs/tags/self-hosted/v0.7.2"},{"ref":"refs/tags/self-hosted/v0.8.0"}]`))
		case "/archive/self-hosted/v0.8.0":
			_, _ = w.Write(archive)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	source, err := New(t.TempDir(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	source.repository, source.archiveBase = server.URL+"/refs", server.URL+"/archive/"
	snapshot, err := source.Resolve(context.Background(), "self-hosted/latest", true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Ref != "self-hosted/v0.8.0" {
		t.Fatalf("ref = %q", snapshot.Ref)
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	writer := tar.NewWriter(gz)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
