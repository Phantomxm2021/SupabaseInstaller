// Package officialtemplate obtains the self-hosted Docker template from the
// Supabase repository.  It deliberately downloads an immutable release tag;
// it never executes upstream shell scripts and it never deploys a branch tip.
package officialtemplate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	defaultRepositoryAPI = "https://api.github.com/repos/supabase/supabase"
	defaultArchiveBase   = "https://codeload.github.com/supabase/supabase/tar.gz/refs/tags/"
	// Codeload archives contain the complete Supabase repository, while only
	// docker/ is extracted below. Keep a bounded download limit, but leave
	// enough headroom for the official repository as it grows.
	maxArchiveBytes         = 128 << 20
	maxExtractedBytes       = 64 << 20
	maxTemplateFiles        = 500
	templateDownloadTimeout = 5 * time.Minute
)

var tagPattern = regexp.MustCompile(`^self-hosted/v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// Snapshot is a validated, immutable copy of the official docker directory.
// Files are relative to that directory and include bootstrap assets as well as
// Compose overlays. SHA256 is the digest of the downloaded archive.
type Snapshot struct {
	Ref       string            `json:"ref"`
	SHA256    string            `json:"sha256"`
	FetchedAt time.Time         `json:"fetchedAt"`
	Files     map[string][]byte `json:"-"`
}

func (s Snapshot) Compose() []byte    { return append([]byte(nil), s.Files["docker-compose.yml"]...) }
func (s Snapshot) EnvExample() []byte { return append([]byte(nil), s.Files[".env.example"]...) }

// Source resolves official template tags and caches verified snapshots on the
// host-mounted project volume. The cache is an audit/recovery artifact, not a
// bundled fallback template.
type Source struct {
	cacheRoot   string
	client      *http.Client
	repository  string
	archiveBase string
}

func New(cacheRoot string, client *http.Client) (*Source, error) {
	if !filepath.IsAbs(cacheRoot) {
		return nil, errors.New("official template cache path must be absolute")
	}
	if client == nil {
		client = &http.Client{Timeout: templateDownloadTimeout}
	}
	return &Source{cacheRoot: cacheRoot, client: client, repository: defaultRepositoryAPI, archiveBase: defaultArchiveBase}, nil
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
		return "", fmt.Errorf("query official Supabase template releases: unexpected HTTP %d", resp.StatusCode)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.archiveBase+ref, nil)
	if err != nil {
		return Snapshot{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("download official Supabase template %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("download official Supabase template %s: unexpected HTTP %d", ref, resp.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read official Supabase template %s: %w", ref, err)
	}
	if len(archive) > maxArchiveBytes {
		return Snapshot{}, errors.New("official Supabase template archive exceeds size limit")
	}
	files, err := extractDockerDirectory(archive)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(archive)
	snapshot := Snapshot{Ref: ref, SHA256: hex.EncodeToString(sum[:]), FetchedAt: time.Now().UTC(), Files: files}
	if err := s.store(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func extractDockerDirectory(archive []byte) (map[string][]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("decode official Supabase template archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	files := map[string][]byte{}
	var total int64
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read official Supabase template archive: %w", readErr)
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		marker := "/docker/"
		index := strings.Index(name, marker)
		if index < 0 || header.Typeflag == tar.TypeDir {
			continue
		}
		relative := strings.TrimPrefix(name[index+len(marker):], "/")
		if relative == "" || strings.HasPrefix(relative, "../") || strings.Contains(relative, "/../") {
			return nil, errors.New("official Supabase template archive contains an unsafe path")
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxExtractedBytes-total {
			return nil, errors.New("official Supabase template archive contains an unsupported file")
		}
		if len(files) >= maxTemplateFiles {
			return nil, errors.New("official Supabase template archive contains too many files")
		}
		data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(data)) != header.Size {
			return nil, errors.New("read official Supabase template archive file")
		}
		total += int64(len(data))
		files[relative] = data
	}
	for _, required := range []string{"docker-compose.yml", ".env.example", "volumes/db/_supabase.sql"} {
		if len(files[required]) == 0 {
			return nil, fmt.Errorf("official Supabase template is missing %s", required)
		}
	}
	return files, nil
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
