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
	"time"

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
	// Lock order is metadataMu -> runtimeMu. Runtime paths never acquire
	// metadataMu, so UpdateMetadata callbacks may safely stage a generation.
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
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

type RuntimeFiles struct {
	Compose      []byte
	Env          []byte
	FunctionsEnv []byte
}

// RuntimePath returns the atomically selected runtime generation. Compose
// callers should use this path as their project directory.
func (r *Root) RuntimePath(slug string) (string, error) {
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectPath, ".manager-runtime", "current"), nil
}

func (r *Root) RuntimeComposePath(slug string) (string, error) {
	path, err := r.RuntimePath(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, "docker-compose.yml"), nil
}
func (r *Root) RuntimeEnvPath(slug string) (string, error) {
	path, err := r.RuntimePath(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, ".env"), nil
}
func (r *Root) RuntimeFunctionsEnvPath(slug string) (string, error) {
	path, err := r.RuntimePath(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, ".env.functions"), nil
}

// StageRuntimeFiles prepares a complete runtime generation. The current
// symlink is the sole publication switch; candidate generations are never
// visible to Compose callers.
func (r *Root) StageRuntimeFiles(slug string, files RuntimeFiles) (restore func() error, commit func() error, err error) {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(projectPath); errors.Is(err, os.ErrNotExist) {
		staging, err := os.MkdirTemp(r.base, "."+slug+"-staging-")
		if err != nil {
			return nil, nil, fmt.Errorf("create project staging directory: %w", err)
		}
		if err := copyEmbeddedTemplate(staging); err != nil {
			_ = os.RemoveAll(staging)
			return nil, nil, err
		}
		if err := os.Rename(staging, projectPath); err != nil {
			_ = os.RemoveAll(staging)
			return nil, nil, fmt.Errorf("publish project directory: %w", err)
		}
	} else if err != nil {
		return nil, nil, fmt.Errorf("inspect project directory: %w", err)
	}

	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "generations"), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create runtime generations: %w", err)
	}
	for _, abandoned := range []string{".candidate-"} {
		matches, _ := filepath.Glob(filepath.Join(runtimeRoot, abandoned+"*"))
		for _, path := range matches {
			_ = os.RemoveAll(path)
		}
	}
	current := filepath.Join(runtimeRoot, "current")
	previousTarget, previousErr := os.Readlink(current)
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read current runtime pointer: %w", previousErr)
	}
	candidateDir, err := os.MkdirTemp(runtimeRoot, ".candidate-")
	if err != nil {
		return nil, nil, fmt.Errorf("create runtime staging directory: %w", err)
	}
	candidate := RuntimeFiles{Compose: append([]byte(nil), files.Compose...), Env: append([]byte(nil), files.Env...), FunctionsEnv: append([]byte(nil), files.FunctionsEnv...)}
	for name, data := range map[string][]byte{"docker-compose.yml": candidate.Compose, ".env": candidate.Env, ".env.functions": candidate.FunctionsEnv} {
		if err := writeAtomic(candidateDir, name, data, 0o600); err != nil {
			_ = os.RemoveAll(candidateDir)
			return nil, nil, err
		}
	}
	if err := syncDirectory(candidateDir); err != nil {
		_ = os.RemoveAll(candidateDir)
		return nil, nil, err
	}
	generationName := fmt.Sprintf("generation-%d", time.Now().UnixNano())
	generationPath := filepath.Join(runtimeRoot, "generations", generationName)
	if err := os.Rename(candidateDir, generationPath); err != nil {
		_ = os.RemoveAll(candidateDir)
		return nil, nil, fmt.Errorf("publish runtime generation: %w", err)
	}

	committed := false
	restored := false
	var restoreLinks func() error
	restore = func() error {
		r.runtimeMu.Lock()
		defer r.runtimeMu.Unlock()
		if restored {
			return nil
		}
		if committed {
			if err := switchRuntimePointer(runtimeRoot, current, previousTarget); err != nil {
				return err
			}
			if previousTarget == "" && restoreLinks != nil {
				if err := restoreLinks(); err != nil {
					return err
				}
			}
		}
		if err := os.RemoveAll(generationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove staged generation: %w", err)
		}
		if err := syncDirectory(filepath.Join(runtimeRoot, "generations")); err != nil {
			return err
		}
		restored = true
		return syncDirectory(projectPath)
	}
	commit = func() error {
		r.runtimeMu.Lock()
		defer r.runtimeMu.Unlock()
		if committed {
			return nil
		}
		var err error
		restoreLinks, err = prepareCompatibilityLinks(projectPath, runtimeRoot)
		if err != nil {
			_ = os.RemoveAll(generationPath)
			return err
		}
		if err := switchRuntimePointer(runtimeRoot, current, generationName); err != nil {
			_ = restoreLinks()
			_ = os.RemoveAll(generationPath)
			return err
		}
		if err := pruneGenerations(filepath.Join(runtimeRoot, "generations"), generationName, previousTarget); err != nil {
			_ = switchRuntimePointer(runtimeRoot, current, previousTarget)
			_ = os.RemoveAll(generationPath)
			return err
		}
		committed = true
		return syncDirectory(runtimeRoot)
	}
	return restore, commit, nil
}

// WriteRuntimeFiles is retained for old callers; new code should stage all
// three runtime files and commit them as one set.
func (r *Root) WriteRuntimeFiles(slug string, compose, environment []byte) error {
	_, commit, err := r.StageRuntimeFiles(slug, RuntimeFiles{Compose: compose, Env: environment})
	if err != nil {
		return err
	}
	if err := commit(); err != nil {
		return err
	}
	return nil
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

func prepareCompatibilityLinks(projectPath, runtimeRoot string) (func() error, error) {
	names := []string{"docker-compose.yml", ".env", ".env.functions"}
	type priorEntry struct {
		existed bool
		symlink bool
		target  string
		backup  string
	}
	prior := make(map[string]priorEntry, len(names))
	restore := func() error {
		for _, name := range names {
			path := filepath.Join(projectPath, name)
			entry := prior[name]
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if !entry.existed {
				continue
			}
			if entry.symlink {
				if err := os.Symlink(entry.target, path); err != nil {
					return err
				}
			} else if err := os.Rename(entry.backup, path); err != nil {
				return err
			}
		}
		return syncDirectory(projectPath)
	}
	for _, name := range names {
		path := filepath.Join(projectPath, name)
		info, err := os.Lstat(path)
		if err == nil {
			entry := priorEntry{existed: true}
			if info.Mode()&os.ModeSymlink != 0 {
				entry.symlink = true
				entry.target, err = os.Readlink(path)
				if err != nil {
					_ = restore()
					return nil, err
				}
			} else {
				entry.backup = filepath.Join(runtimeRoot, ".legacy-"+name+"-"+fmt.Sprint(time.Now().UnixNano()))
				if err := os.Rename(path, entry.backup); err != nil {
					_ = restore()
					return nil, err
				}
			}
			prior[name] = entry
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		target := filepath.Join(".manager-runtime", "current", name)
		temp := path + ".link-tmp"
		if entry, ok := prior[name]; ok && entry.symlink && entry.target == target {
			continue
		}
		if entry, ok := prior[name]; ok && entry.symlink {
			if err := os.Remove(path); err != nil {
				_ = restore()
				return nil, err
			}
		}
		if err := os.Symlink(target, temp); err != nil {
			_ = restore()
			return nil, err
		}
		if err := os.Rename(temp, path); err != nil {
			_ = os.Remove(temp)
			_ = restore()
			return nil, err
		}
	}
	return restore, nil
}

func switchRuntimePointer(runtimeRoot, current, target string) error {
	if target == "" {
		if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return syncDirectory(runtimeRoot)
	}
	target = strings.TrimPrefix(filepath.ToSlash(target), "generations/")
	temporary := filepath.Join(runtimeRoot, fmt.Sprintf(".current-%d", time.Now().UnixNano()))
	if err := os.Symlink(filepath.Join("generations", target), temporary); err != nil {
		return fmt.Errorf("create runtime pointer: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("switch runtime pointer: %w", err)
	}
	return syncDirectory(runtimeRoot)
}

func pruneGenerations(directory, current, previous string) error {
	previous = strings.TrimPrefix(filepath.ToSlash(previous), "generations/")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current || entry.Name() == previous {
			continue
		}
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return syncDirectory(directory)
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

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}

type Root struct {
	base       string
	metadataMu sync.Mutex
	runtimeMu  sync.Mutex
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
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	return r.readMetadata(slug)
}

func (r *Root) UpdateMetadata(slug string, mutate func(*Metadata) error) (Metadata, error) {
	// Keep revision/idempotency mutation serialized across the complete
	// callback. The callback may acquire runtimeMu to publish a generation;
	// runtime code must never acquire metadataMu.
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
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
