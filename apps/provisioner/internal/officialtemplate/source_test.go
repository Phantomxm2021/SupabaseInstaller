package officialtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNewAllowsFiveMinutesForOfficialTemplateDownloads(t *testing.T) {
	source, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if source.client.Timeout < 5*time.Minute {
		t.Fatalf("official template timeout = %s, want at least 5m", source.client.Timeout)
	}
}

func TestResolveReadsDockerFilesFromOfficialGitAPIAndCachesRelease(t *testing.T) {
	files := fixtureDockerFiles()
	server, calls := newOfficialGitServer(t, files, nil)
	defer server.Close()

	source, err := New(t.TempDir(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	source.repository, source.rawBase = server.URL, server.URL+"/raw"
	first, err := source.Resolve(context.Background(), "self-hosted/v9.8.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ref != "self-hosted/v9.8.7" || len(first.SHA256) != 64 || string(first.Compose()) != "services: {}\n" {
		t.Fatalf("unexpected snapshot: %#v", first)
	}
	second, err := source.Resolve(context.Background(), "self-hosted/v9.8.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if second.SHA256 != first.SHA256 {
		t.Fatalf("cached digest = %s, want %s", second.SHA256, first.SHA256)
	}
	if got := calls["/git/ref/tags/self-hosted/v9.8.7"]; got != 1 {
		t.Fatalf("tag ref requests = %d, want 1 after cache reload", got)
	}
}

func TestResolveLatestUsesGreatestOfficialSelfHostedTag(t *testing.T) {
	server, _ := newOfficialGitServer(t, fixtureDockerFiles(), []string{"self-hosted/v0.7.2", "self-hosted/v0.8.0"})
	defer server.Close()
	source, err := New(t.TempDir(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	source.repository, source.rawBase = server.URL, server.URL+"/raw"
	snapshot, err := source.Resolve(context.Background(), "self-hosted/latest", true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Ref != "self-hosted/v0.8.0" {
		t.Fatalf("ref = %q", snapshot.Ref)
	}
}

func fixtureDockerFiles() map[string][]byte {
	return map[string][]byte{
		"docker/docker-compose.yml":       []byte("services: {}\n"),
		"docker/.env.example":             []byte("POSTGRES_PASSWORD=example\n"),
		"docker/volumes/db/_supabase.sql": []byte("select 1;\n"),
	}
}

func newOfficialGitServer(t *testing.T, files map[string][]byte, latestRefs []string) (*httptest.Server, map[string]int) {
	t.Helper()
	const commitSHA = "1111111111111111111111111111111111111111"
	const treeSHA = "2222222222222222222222222222222222222222"
	const tagSHA = "3333333333333333333333333333333333333333"
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	blobSHA := make(map[string]string, len(files))
	for index, name := range names {
		blobSHA[fmt.Sprintf("%040x", index+1)] = name
	}
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		switch r.URL.Path {
		case "/git/matching-refs/tags/self-hosted/":
			refs := make([]map[string]string, 0, len(latestRefs))
			for _, ref := range latestRefs {
				refs = append(refs, map[string]string{"ref": "refs/tags/" + ref})
			}
			_ = json.NewEncoder(w).Encode(refs)
		case "/git/ref/tags/self-hosted/v9.8.7", "/git/ref/tags/self-hosted/v0.8.0":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": tagSHA, "type": "tag"}})
		case "/git/tags/" + tagSHA:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": commitSHA, "type": "commit"}})
		case "/git/commits/" + commitSHA:
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]string{"sha": treeSHA}})
		case "/git/trees/" + treeSHA:
			if r.URL.Query().Get("recursive") != "1" {
				t.Fatalf("recursive = %q", r.URL.Query().Get("recursive"))
			}
			entries := make([]map[string]any, 0, len(files))
			for sha, path := range blobSHA {
				entries = append(entries, map[string]any{"path": path, "type": "blob", "sha": sha, "size": len(files[path])})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"truncated": false, "tree": entries})
		default:
			if name := strings.TrimPrefix(r.URL.Path, "/raw/"+commitSHA+"/"); name != r.URL.Path {
				if content, ok := files[name]; ok {
					_, _ = w.Write(content)
					return
				}
			}
			t.Fatalf("unexpected request %s", r.URL)
		}
	}))
	return server, calls
}
