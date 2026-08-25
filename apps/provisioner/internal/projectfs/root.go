package projectfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"supabase-manager/internal/templates"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var ErrNotFound = errors.New("project metadata not found")

type Metadata struct {
	ProjectID   string                     `json:"projectId"`
	ProjectName string                     `json:"projectName"`
	Slug        string                     `json:"slug"`
	Revision    int64                      `json:"revision"`
	Idempotency map[string]json.RawMessage `json:"idempotency"`
}

func (r *Root) DeleteProjectData(slug string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return err
	}
	if projectPath == r.base {
		return fmt.Errorf("refusing to remove project root")
	}
	if err := os.RemoveAll(projectPath); err != nil {
		return fmt.Errorf("remove confirmed project data: %w", err)
	}
	return nil
}

func (r *Root) WriteRuntimeFiles(slug string, compose, environment []byte) error {
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(projectPath); errors.Is(err, os.ErrNotExist) {
		staging, err := os.MkdirTemp(r.base, "."+slug+"-staging-")
		if err != nil {
			return fmt.Errorf("create project staging directory: %w", err)
		}
		defer os.RemoveAll(staging)
		if err := copyEmbeddedTemplate(staging); err != nil {
			return err
		}
		if err := writeAtomic(staging, "docker-compose.yml", compose, 0o600); err != nil {
			return err
		}
		if err := writeAtomic(staging, ".env", environment, 0o600); err != nil {
			return err
		}
		if err := os.Rename(staging, projectPath); err != nil {
			return fmt.Errorf("publish project directory: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect project directory: %w", err)
	}
	if err := writeAtomic(projectPath, "docker-compose.yml", compose, 0o600); err != nil {
		return err
	}
	return writeAtomic(projectPath, ".env", environment, 0o600)
}

func copyEmbeddedTemplate(destination string) error {
	templateFS := templates.Files()
	const sourceRoot = "self-hosted-v0.8.0"
	return fs.WalkDir(templateFS, sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative := strings.TrimPrefix(path, sourceRoot)
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded template file %s: %w", path, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("read embedded template mode %s: %w", path, err)
		}
		mode := fs.FileMode(0o600)
		if info.Mode()&0o111 != 0 {
			mode = 0o700
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return fmt.Errorf("write template file %s: %w", relative, err)
		}
		return nil
	})
}

func writeAtomic(directory, name string, data []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(name)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", name, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set %s permissions: %w", name, err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync %s: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(temporaryName, filepath.Join(directory, name)); err != nil {
		return fmt.Errorf("publish %s: %w", name, err)
	}
	return nil
}

type Root struct {
	base string
	mu   sync.Mutex
}

func New(base string) (*Root, error) {
	if base == "" {
		return nil, fmt.Errorf("project root is required")
	}
	absolute, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create project root: %w", err)
	}
	return &Root{base: filepath.Clean(absolute)}, nil
}

func (r *Root) ProjectPath(slug string) (string, error) {
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid project slug")
	}
	candidate := filepath.Join(r.base, slug)
	relative, err := filepath.Rel(r.base, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("project path escapes configured root")
	}
	return candidate, nil
}

func (r *Root) Metadata(slug string) (Metadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readMetadata(slug)
}

func (r *Root) UpdateMetadata(slug string, mutate func(*Metadata) error) (Metadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	metadata, err := r.readMetadata(slug)
	if errors.Is(err, ErrNotFound) {
		metadata = Metadata{Slug: slug, Idempotency: make(map[string]json.RawMessage)}
	} else if err != nil {
		return Metadata{}, err
	}
	if metadata.Idempotency == nil {
		metadata.Idempotency = make(map[string]json.RawMessage)
	}
	if err := mutate(&metadata); err != nil {
		return Metadata{}, err
	}
	if err := r.writeMetadata(slug, metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func (r *Root) readMetadata(slug string) (Metadata, error) {
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(filepath.Join(projectPath, "project.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("read project metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode project metadata: %w", err)
	}
	return metadata, nil
}

func (r *Root) writeMetadata(slug string, metadata Metadata) error {
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project metadata: %w", err)
	}
	temporary, err := os.CreateTemp(projectPath, ".project.json-*")
	if err != nil {
		return fmt.Errorf("create temporary metadata: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure metadata permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}
	if err := os.Rename(temporaryName, filepath.Join(projectPath, "project.json")); err != nil {
		return fmt.Errorf("publish metadata: %w", err)
	}
	return nil
}
