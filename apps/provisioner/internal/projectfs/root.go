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

// RuntimeRef describes the stable Compose project directory and the current
// atomically selected generated configuration files.
type RuntimeRef struct {
	ProjectDir    string
	ComposeFile   string
	EnvFile       string
	FunctionsFile string
}

// RuntimePath returns the stable project/data root. Compose must use this as
// --project-directory so relative volume paths always resolve persistent data.
func (r *Root) RuntimePath(slug string) (string, error) {
	return r.ProjectPath(slug)
}

func (r *Root) CurrentRuntimeFiles(slug string) (RuntimeRef, error) {
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return RuntimeRef{}, err
	}
	current := filepath.Join(projectPath, ".manager-runtime", "current")
	return RuntimeRef{ProjectDir: projectPath, ComposeFile: filepath.Join(current, "docker-compose.yml"), EnvFile: filepath.Join(current, ".env"), FunctionsFile: filepath.Join(current, ".env.functions")}, nil
}

func (r *Root) RuntimeComposePath(slug string) (string, error) {
	ref, err := r.CurrentRuntimeFiles(slug)
	if err != nil {
		return "", err
	}
	return ref.ComposeFile, nil
}
func (r *Root) RuntimeEnvPath(slug string) (string, error) {
	ref, err := r.CurrentRuntimeFiles(slug)
	if err != nil {
		return "", err
	}
	return ref.EnvFile, nil
}
func (r *Root) RuntimeFunctionsEnvPath(slug string) (string, error) {
	ref, err := r.CurrentRuntimeFiles(slug)
	if err != nil {
		return "", err
	}
	return ref.FunctionsFile, nil
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
	current := filepath.Join(runtimeRoot, "current")
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

	committed := false
	restored := false
	var committedTarget string
	var previousTarget string
	restore = func() error {
		r.runtimeMu.Lock()
		defer r.runtimeMu.Unlock()
		if restored {
			return nil
		}
		if committed {
			active, err := os.Readlink(current)
			if err != nil {
				return fmt.Errorf("read current runtime pointer: %w", err)
			}
			if filepath.ToSlash(strings.TrimPrefix(active, "generations/")) != committedTarget {
				return fmt.Errorf("stale runtime generation %s", committedTarget)
			}
			if err := switchRuntimePointer(runtimeRoot, current, previousTarget); err != nil {
				return err
			}
		}
		stagedPath := generationPath
		if !committed {
			stagedPath = candidateDir
		}
		if err := os.RemoveAll(stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
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
		if err := os.Rename(candidateDir, generationPath); err != nil {
			return fmt.Errorf("publish runtime generation: %w", err)
		}
		active, err := os.Readlink(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.RemoveAll(generationPath)
			return fmt.Errorf("read current runtime pointer: %w", err)
		}
		previousTarget = active
		if err := switchRuntimePointer(runtimeRoot, current, generationName); err != nil {
			_ = os.RemoveAll(generationPath)
			return err
		}
		committedTarget = generationName
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

// CleanupAbandonedRuntimeCandidates removes pre-commit candidate directories
// left by an interrupted process. It is separate from staging so concurrent
// stage closures cannot delete one another's candidates.
func (r *Root) CleanupAbandonedRuntimeCandidates(slug string) error {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return err
	}
	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	matches, err := filepath.Glob(filepath.Join(runtimeRoot, ".candidate-*"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove abandoned runtime candidate: %w", err)
		}
	}
	return nil
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
		switch relative {
		case "docker-compose.yml", ".env", ".env.functions":
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
