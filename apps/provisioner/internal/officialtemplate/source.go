// Package officialtemplate obtains the self-hosted Docker template from the
// Supabase repository.  It deliberately downloads an immutable release tag;
// it never executes upstream shell scripts and it never deploys a branch tip.
package officialtemplate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRepositoryAPI    = "https://api.github.com/repos/supabase/supabase"
	defaultRawBase          = "https://raw.githubusercontent.com/supabase/supabase"
	maxExtractedBytes       = 64 << 20
	maxTemplateFiles        = 500
	templateDownloadTimeout = 5 * time.Minute
)

var tagPattern = regexp.MustCompile(`^self-hosted/v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// Snapshot is a validated, immutable copy of the official docker directory.
// Files are relative to that directory and include bootstrap assets as well as
// Compose overlays. SHA256 is the digest of the verified template file set.
type Snapshot struct {
	Ref       string            `json:"ref"`
	SHA256    string            `json:"sha256"`
	FetchedAt time.Time         `json:"fetchedAt"`
	Files     map[string][]byte `json:"-"`
}

// RateLimitError is a safe, typed GitHub API failure. It contains only the
// server-provided reset time and never copies the response body.
type RateLimitError struct {
	Reset time.Time
}

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "GitHub API rate limit exceeded"
	}
	return "GitHub API rate limit exceeded until " + e.Reset.UTC().Format(time.RFC3339)
}

func (s Snapshot) Compose() []byte    { return append([]byte(nil), s.Files["docker-compose.yml"]...) }
func (s Snapshot) EnvExample() []byte { return append([]byte(nil), s.Files[".env.example"]...) }

// Source resolves official template tags and caches verified snapshots on the
// host-mounted project volume. The cache is an audit/recovery artifact, not a
// bundled fallback template.
type Source struct {
	cacheRoot  string
	client     *http.Client
	repository string
	rawBase    string
}

func New(cacheRoot string, client *http.Client) (*Source, error) {
	if !filepath.IsAbs(cacheRoot) {
		return nil, errors.New("official template cache path must be absolute")
	}
	if client == nil {
		client = &http.Client{Timeout: templateDownloadTimeout}
	}
	return &Source{cacheRoot: cacheRoot, client: client, repository: defaultRepositoryAPI, rawBase: defaultRawBase}, nil
}

// Resolve returns the cache for a previously applied tag unless refresh is
// requested. A refresh asks GitHub for the newest self-hosted release tag.
func (s *Source) Resolve(ctx context.Context, ref string, refresh bool) (Snapshot, error) {
	if ref == "" || ref == "self-hosted/latest" {
		var err error
		ref, err = s.latestTag(ctx)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if !tagPattern.MatchString(ref) {
		return Snapshot{}, fmt.Errorf("official template ref %q is not a self-hosted release tag", ref)
	}
	if !refresh {
		if cached, err := s.load(ref); err == nil {
			return cached, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, err
		}
	}
	return s.download(ctx, ref)
}

type githubRef struct {
	Ref string `json:"ref"`
}

func (s *Source) latestTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.repository+"/git/matching-refs/tags/self-hosted/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query official Supabase template releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("query official Supabase template releases: %w", githubStatusError(resp))
	}
	var refs []githubRef
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&refs); err != nil {
		return "", fmt.Errorf("decode official Supabase template releases: %w", err)
	}
	latest := ""
	for _, item := range refs {
		candidate := strings.TrimPrefix(item.Ref, "refs/tags/")
		if !tagPattern.MatchString(candidate) || versionAtLeast(latest, candidate) {
			continue
		}
		latest = candidate
	}
	if latest == "" {
		return "", errors.New("official Supabase repository returned no self-hosted release tags")
	}
	return latest, nil
}

func versionAtLeast(left, right string) bool {
	if left == "" {
		return false
	}
	l, r := tagPattern.FindStringSubmatch(left), tagPattern.FindStringSubmatch(right)
	for index := 1; index <= 3; index++ {
		lv, _ := strconv.Atoi(l[index])
		rv, _ := strconv.Atoi(r[index])
		if lv != rv {
			return lv > rv
		}
	}
	return true
}

func (s *Source) download(ctx context.Context, ref string) (Snapshot, error) {
	commit, err := s.releaseCommit(ctx, ref)
	if err != nil {
		return Snapshot{}, err
	}
	files, err := s.dockerFiles(ctx, commit)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Ref: ref, SHA256: filesDigest(files), FetchedAt: time.Now().UTC(), Files: files}
	if err := s.store(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

type gitObject struct{ SHA, Type string }

type gitRefResponse struct {
	Object gitObject `json:"object"`
}
type gitTagResponse struct {
	Object gitObject `json:"object"`
}
type gitCommitResponse struct {
	Tree struct {
		SHA string `json:"sha"`
	} `json:"tree"`
}
type gitTreeEntry struct {
	Path, Type, SHA string
	Size            int64
}
type gitTreeResponse struct {
	Truncated bool           `json:"truncated"`
	Tree      []gitTreeEntry `json:"tree"`
}

func (s *Source) releaseCommit(ctx context.Context, ref string) (string, error) {
	var tag gitRefResponse
	if err := s.getJSON(ctx, "/git/ref/tags/"+ref, 1<<20, &tag); err != nil {
		return "", fmt.Errorf("read official Supabase template ref %s: %w", ref, err)
	}
	object := tag.Object
	for depth := 0; object.Type == "tag" && depth < 4; depth++ {
		var annotated gitTagResponse
		if err := s.getJSON(ctx, "/git/tags/"+object.SHA, 1<<20, &annotated); err != nil {
			return "", fmt.Errorf("read official Supabase template tag %s: %w", ref, err)
		}
		object = annotated.Object
	}
	if object.Type != "commit" || len(object.SHA) != 40 {
		return "", fmt.Errorf("official Supabase template ref %s does not resolve to a commit", ref)
	}
	return object.SHA, nil
}

func (s *Source) dockerFiles(ctx context.Context, commit string) (map[string][]byte, error) {
	var commitResponse gitCommitResponse
	if err := s.getJSON(ctx, "/git/commits/"+commit, 1<<20, &commitResponse); err != nil {
		return nil, fmt.Errorf("read official Supabase template commit: %w", err)
	}
	if len(commitResponse.Tree.SHA) != 40 {
		return nil, errors.New("official Supabase template commit has no tree")
	}
	var tree gitTreeResponse
	if err := s.getJSON(ctx, "/git/trees/"+commitResponse.Tree.SHA+"?recursive=1", 8<<20, &tree); err != nil {
		return nil, fmt.Errorf("read official Supabase template tree: %w", err)
	}
	if tree.Truncated {
		return nil, errors.New("official Supabase template tree response is incomplete")
	}
	entries := make([]gitTreeEntry, 0)
	var total int64
	for _, entry := range tree.Tree {
		if entry.Type != "blob" || !strings.HasPrefix(entry.Path, "docker/") {
			continue
		}
		entry.Path = strings.TrimPrefix(entry.Path, "docker/")
		if !safeName(entry.Path) || entry.Size < 0 || entry.Size > maxExtractedBytes-total || len(entry.SHA) != 40 {
			return nil, errors.New("official Supabase template tree contains an unsafe file")
		}
		if len(entries) >= maxTemplateFiles {
			return nil, errors.New("official Supabase template tree contains too many files")
		}
		total += entry.Size
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Path < entries[right].Path })
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		data, err := s.getRaw(ctx, commit, "docker/"+entry.Path, entry.Size)
		if err != nil || int64(len(data)) != entry.Size {
			return nil, fmt.Errorf("official Supabase template file %s has invalid content", entry.Path)
		}
		files[entry.Path] = data
	}
	for _, required := range []string{"docker-compose.yml", ".env.example", "volumes/db/_supabase.sql"} {
		if len(files[required]) == 0 {
			return nil, fmt.Errorf("official Supabase template is missing %s", required)
		}
	}
	return files, nil
}

func (s *Source) getRaw(ctx context.Context, commit, path string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.rawBase+"/"+commit+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("response exceeds size limit")
	}
	return body, nil
}

func (s *Source) getJSON(ctx context.Context, path string, maxBytes int64, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.repository+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return githubStatusError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxBytes {
		return errors.New("response exceeds size limit")
	}
	return json.Unmarshal(body, target)
}

func githubStatusError(resp *http.Response) error {
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		resetUnix, _ := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
		limited := &RateLimitError{}
		if resetUnix > 0 {
			limited.Reset = time.Unix(resetUnix, 0).UTC()
		}
		return limited
	}
	return fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
}

func filesDigest(files map[string][]byte) string {
	hash := sha256.New()
	for _, name := range (Snapshot{Files: files}).SortedNames() {
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(files[name])
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Source) path(ref string) string {
	return filepath.Join(s.cacheRoot, strings.ReplaceAll(ref, "/", "_"))
}

func (s *Source) store(snapshot Snapshot) error {
	destination := s.path(snapshot.Ref)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return fmt.Errorf("create official template cache: %w", err)
	}
	manifest, err := json.Marshal(struct {
		Ref       string    `json:"ref"`
		SHA256    string    `json:"sha256"`
		FetchedAt time.Time `json:"fetchedAt"`
	}{snapshot.Ref, snapshot.SHA256, snapshot.FetchedAt})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destination, "manifest.json"), manifest, 0o600); err != nil {
		return fmt.Errorf("write official template manifest: %w", err)
	}
	for name, data := range snapshot.Files {
		if !safeName(name) {
			return errors.New("refusing to cache unsafe official template path")
		}
		path := filepath.Join(destination, "files", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return fmt.Errorf("cache official template file: %w", err)
		}
	}
	return nil
}

func (s *Source) load(ref string) (Snapshot, error) {
	manifest, err := os.ReadFile(filepath.Join(s.path(ref), "manifest.json"))
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(manifest, &snapshot); err != nil || snapshot.Ref != ref || len(snapshot.SHA256) != 64 {
		return Snapshot{}, errors.New("official template cache manifest is invalid")
	}
	root := filepath.Join(s.path(ref), "files")
	files := map[string][]byte{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || !safeName(filepath.ToSlash(relative)) {
			return errors.New("official template cache contains an unsafe path")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Files = files
	if len(snapshot.Compose()) == 0 || len(snapshot.EnvExample()) == 0 || len(files["volumes/db/_supabase.sql"]) == 0 {
		return Snapshot{}, errors.New("official template cache is incomplete")
	}
	return snapshot, nil
}

func safeName(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))
	return name != "." && !strings.HasPrefix(name, "../") && !strings.Contains(name, "/../") && !filepath.IsAbs(name)
}

// SortedNames is useful when serializing a snapshot in deterministic tests.
func (s Snapshot) SortedNames() []string {
	names := make([]string, 0, len(s.Files))
	for name := range s.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
