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

	"supabase-manager/internal/contracts"
	"supabase-manager/internal/templates"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var ErrNotFound = errors.New("project metadata not found")

type Metadata struct {
	ProjectID       string                         `json:"projectId"`
	ProjectName     string                         `json:"projectName"`
	Slug            string                         `json:"slug"`
	Revision        int64                          `json:"revision"`
	Idempotency     map[string]json.RawMessage     `json:"idempotency"`
	Configuration   contracts.ProjectConfiguration `json:"configuration,omitempty"`
	EnabledServices []string                       `json:"enabledServices,omitempty"`
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

// legacyRuntimeNames are the only runtime artifacts ever written by older
// Manager versions at the project root. All other root entries belong to the
// user's project data and must remain untouched during migration.
var legacyRuntimeNames = []string{"docker-compose.yml", ".env", ".env.functions"}

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

// CurrentRuntimeGeneration returns an immutable ref to the currently selected
// generation. Unlike CurrentRuntimeFiles, its paths do not follow the current
// symlink after a later commit.
func (r *Root) CurrentRuntimeGeneration(slug string) (RuntimeRef, error) {
	ref, err := r.CurrentRuntimeFiles(slug)
	if err != nil { return RuntimeRef{}, err }
	target, err := os.Readlink(filepath.Join(ref.ProjectDir, ".manager-runtime", "current"))
	if errors.Is(err, os.ErrNotExist) { return ref, nil }
	if err != nil { return RuntimeRef{}, err }
	generation := filepath.Join(ref.ProjectDir, ".manager-runtime", filepath.FromSlash(target))
	return RuntimeRef{ProjectDir: ref.ProjectDir, ComposeFile: filepath.Join(generation, "docker-compose.yml"), EnvFile: filepath.Join(generation, ".env"), FunctionsFile: filepath.Join(generation, ".env.functions")}, nil
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
	_, restore, commit, err = r.StageRuntimeFilesWithRef(slug, files)
	return restore, commit, err
}

// StageRuntimeFilesWithRef exposes candidate paths while keeping the stable
// project directory. The candidate is unpublished until commit succeeds.
func (r *Root) StageRuntimeFilesWithRef(slug string, files RuntimeFiles) (candidate RuntimeRef, restore func() error, commit func() error, err error) {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	projectPath, err := r.ProjectPath(slug)
	if err != nil {
		return RuntimeRef{}, nil, nil, err
	}
	if _, err := os.Stat(projectPath); errors.Is(err, os.ErrNotExist) {
		staging, err := os.MkdirTemp(r.base, "."+slug+"-staging-")
		if err != nil {
			return RuntimeRef{}, nil, nil, fmt.Errorf("create project staging directory: %w", err)
		}
		if err := copyEmbeddedTemplate(staging); err != nil {
			_ = os.RemoveAll(staging)
			return RuntimeRef{}, nil, nil, err
		}
		if err := os.Rename(staging, projectPath); err != nil {
			_ = os.RemoveAll(staging)
			return RuntimeRef{}, nil, nil, fmt.Errorf("publish project directory: %w", err)
		}
	} else if err != nil {
		return RuntimeRef{}, nil, nil, fmt.Errorf("inspect project directory: %w", err)
	}
	legacyFiles, err := identifyLegacyRuntimeFiles(projectPath)
	if err != nil {
		return RuntimeRef{}, nil, nil, err
	}

	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "generations"), 0o700); err != nil {
		return RuntimeRef{}, nil, nil, fmt.Errorf("create runtime generations: %w", err)
	}
	current := filepath.Join(runtimeRoot, "current")
	candidateDir, err := os.MkdirTemp(runtimeRoot, ".candidate-")
	if err != nil {
		return RuntimeRef{}, nil, nil, fmt.Errorf("create runtime staging directory: %w", err)
	}
	candidateFiles := RuntimeFiles{Compose: append([]byte(nil), files.Compose...), Env: append([]byte(nil), files.Env...), FunctionsEnv: append([]byte(nil), files.FunctionsEnv...)}
	for name, data := range map[string][]byte{"docker-compose.yml": candidateFiles.Compose, ".env": candidateFiles.Env, ".env.functions": candidateFiles.FunctionsEnv} {
		if err := writeAtomic(candidateDir, name, data, 0o600); err != nil {
			_ = os.RemoveAll(candidateDir)
			return RuntimeRef{}, nil, nil, err
		}
	}
	if err := r.syncRuntimeDirectory(candidateDir); err != nil {
		_ = os.RemoveAll(candidateDir)
		return RuntimeRef{}, nil, nil, err
	}
	generationName := fmt.Sprintf("generation-%d", time.Now().UnixNano())
	generationPath := filepath.Join(runtimeRoot, "generations", generationName)

	committed := false
	restored := false
	generationPublished := false
	var committedTarget string
	var previousTarget string
	var legacyCleanup *legacyRuntimeCleanup
	restore = func() error {
		r.runtimeMu.Lock()
		defer r.runtimeMu.Unlock()
		if restored {
			return nil
		}
		// A failed legacy migration must be rolled back before changing the
		// current pointer. This keeps the returned restore closure coherent even
		// when cleanup failed after publication.
		if legacyCleanup != nil {
			if err := legacyCleanup.rollback(); err != nil {
				return err
			}
		}
		if committed {
			active, err := os.Readlink(current)
			if err != nil {
				return fmt.Errorf("read current runtime pointer: %w", err)
			}
			if filepath.ToSlash(strings.TrimPrefix(active, "generations/")) != committedTarget {
				return fmt.Errorf("stale runtime generation %s", committedTarget)
			}
			if err := r.switchRuntimePointer(runtimeRoot, current, previousTarget); err != nil {
				return err
			}
		}
		stagedPath := generationPath
		if !generationPublished {
			stagedPath = candidateDir
		}
		if err := os.RemoveAll(stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove staged generation: %w", err)
		}
		if err := r.syncRuntimeDirectory(filepath.Join(runtimeRoot, "generations")); err != nil {
			return err
		}
		restored = true
		return r.syncRuntimeDirectory(projectPath)
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
		generationPublished = true
		r.recordRuntimeOperation("rename-generation")
		// The generation directory entry must be durable before current can
		// point at it. Otherwise a crash can leave a durable pointer to a missing
		// generation.
		generationsDir := filepath.Join(runtimeRoot, "generations")
		if err := r.syncRuntimeDirectory(generationsDir); err != nil {
			return err
		}
		active, err := os.Readlink(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read current runtime pointer: %w", err)
		}
		previousTarget = active
		if err := r.switchRuntimePointer(runtimeRoot, current, generationName); err != nil {
			return err
		}
		committedTarget = generationName
		// Keep every committed generation while restore closures may still
		// refer to it. Chained rollback (A -> B -> C -> B -> A) depends on
		// ancestors remaining valid; startup cleanup only removes candidates.
		// Mark this before cleanup so restore knows that current must be switched
		// back if moving a legacy entry fails.
		committed = true
		legacyCleanup, err = r.cleanupLegacyRuntimeFiles(projectPath, runtimeRoot, legacyFiles)
		if err != nil {
			return err
		}
		return nil
	}
	candidate = RuntimeRef{ProjectDir: projectPath, ComposeFile: filepath.Join(candidateDir, "docker-compose.yml"), EnvFile: filepath.Join(candidateDir, ".env"), FunctionsFile: filepath.Join(candidateDir, ".env.functions")}
	return candidate, restore, commit, nil
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
	return cleanupAbandonedRuntimeCandidates(projectPath)
}

func cleanupAbandonedRuntimeCandidates(projectPath string) error {
	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	matches, err := filepath.Glob(filepath.Join(runtimeRoot, ".candidate-*"))
	if err != nil {
		return err
	}
	removed := false
	for _, path := range matches {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove abandoned runtime candidate: %w", err)
		}
		removed = true
	}
	if removed {
		if err := syncDirectory(runtimeRoot); err != nil {
			return err
		}
	}
	return cleanupAbandonedLegacyQuarantines(projectPath)
}

// cleanupAbandonedLegacyQuarantines repairs interrupted compatibility-file
// migrations. A quarantine with a valid current generation is safe to discard;
// without a valid current generation its exact legacy entries are restored to
// the project root before the quarantine is removed.
func cleanupAbandonedLegacyQuarantines(projectPath string) error {
	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	matches, err := filepath.Glob(filepath.Join(runtimeRoot, ".legacy-quarantine-*"))
	if err != nil {
		return err
	}
	for _, quarantine := range matches {
		current := filepath.Join(runtimeRoot, "current")
		target, targetErr := os.Readlink(current)
		currentValid := targetErr == nil && target != ""
		if currentValid {
			if _, err := os.Stat(current); err != nil {
				currentValid = false
			}
		}
		if !currentValid {
			for _, name := range legacyRuntimeNames {
				source := filepath.Join(quarantine, name)
				if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
					continue
				} else if err != nil {
					return fmt.Errorf("inspect abandoned legacy quarantine %s: %w", name, err)
				}
				destination := filepath.Join(projectPath, name)
				if _, err := os.Lstat(destination); err == nil {
					// Never overwrite a project file during conservative recovery;
					// retain the quarantine for an operator to resolve explicitly.
					return fmt.Errorf("legacy recovery conflict at %s", name)
				} else if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("inspect legacy recovery destination %s: %w", name, err)
				}
				if err := os.Rename(source, destination); err != nil {
					return fmt.Errorf("recover legacy runtime file %s: %w", name, err)
				}
			}
		}
		if err := os.RemoveAll(quarantine); err != nil {
			return fmt.Errorf("remove abandoned legacy quarantine: %w", err)
		}
		if err := syncDirectory(projectPath); err != nil {
			return err
		}
		if err := syncDirectory(runtimeRoot); err != nil {
			return err
		}
	}
	return nil
}

// WriteRuntimeFiles is retained for old callers; new code should stage all
// three runtime files and commit them as one set.
func (r *Root) WriteRuntimeFiles(slug string, compose, environment []byte) error {
	restore, commit, err := r.StageRuntimeFiles(slug, RuntimeFiles{Compose: compose, Env: environment})
	if err != nil {
		return err
	}
	if err := commit(); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
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
	return switchRuntimePointerWithOps(runtimeRoot, current, target, syncDirectory, nil)
}

func switchRuntimePointerWithOps(runtimeRoot, current, target string, syncDir func(string) error, operation func(string)) error {
	previousTarget, err := os.Readlink(current)
	previousPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current runtime pointer: %w", err)
	}
	target = strings.TrimPrefix(filepath.ToSlash(target), "generations/")
	if target == "" {
		if !previousPresent {
			return nil
		}
		if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove runtime pointer: %w", err)
		}
		if operation != nil {
			operation("remove-current")
		}
		if err := syncDir(runtimeRoot); err != nil {
			if rollbackErr := restoreRuntimePointer(runtimeRoot, current, previousTarget, previousPresent, syncDir, operation); rollbackErr != nil {
				return fmt.Errorf("sync removed runtime pointer: %w; restore previous pointer: %v", err, rollbackErr)
			}
			return fmt.Errorf("sync removed runtime pointer: %w", err)
		}
		return nil
	}
	temporary := filepath.Join(runtimeRoot, fmt.Sprintf(".current-%d", time.Now().UnixNano()))
	if err := os.Symlink(filepath.Join("generations", target), temporary); err != nil {
		return fmt.Errorf("create runtime pointer: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("switch runtime pointer: %w", err)
	}
	if operation != nil {
		operation("rename-current")
	}
	if err := syncDir(runtimeRoot); err != nil {
		if rollbackErr := restoreRuntimePointer(runtimeRoot, current, previousTarget, previousPresent, syncDir, operation); rollbackErr != nil {
			return fmt.Errorf("sync runtime pointer: %w; restore previous pointer: %v", err, rollbackErr)
		}
		return fmt.Errorf("sync runtime pointer: %w", err)
	}
	return nil
}

func restoreRuntimePointer(runtimeRoot, current, target string, targetPresent bool, syncDir func(string) error, operation func(string)) error {
	if !targetPresent || target == "" {
		if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove runtime pointer during rollback: %w", err)
		}
		if operation != nil {
			operation("remove-current-rollback")
		}
		return syncDir(runtimeRoot)
	}
	target = strings.TrimPrefix(filepath.ToSlash(target), "generations/")
	temporary := filepath.Join(runtimeRoot, fmt.Sprintf(".current-rollback-%d", time.Now().UnixNano()))
	if err := os.Symlink(filepath.Join("generations", target), temporary); err != nil {
		return fmt.Errorf("create rollback runtime pointer: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("restore runtime pointer: %w", err)
	}
	if operation != nil {
		operation("rename-current-rollback")
	}
	if err := syncDir(runtimeRoot); err != nil {
		return fmt.Errorf("sync restored runtime pointer: %w", err)
	}
	return nil
}

func identifyLegacyRuntimeFiles(projectPath string) ([]string, error) {
	identified := make([]string, 0, len(legacyRuntimeNames))
	for _, name := range legacyRuntimeNames {
		info, err := os.Lstat(filepath.Join(projectPath, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect legacy runtime file %s: %w", name, err)
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			identified = append(identified, name)
		}
	}
	return identified, nil
}

func removeLegacyRuntimeFiles(projectPath string, names []string) error {
	runtimeRoot := filepath.Join(projectPath, ".manager-runtime")
	cleanup, err := (&Root{}).cleanupLegacyRuntimeFiles(projectPath, runtimeRoot, names)
	if err != nil {
		return err
	}
	if cleanup != nil && !cleanup.finalized {
		return cleanup.rollback()
	}
	return nil
}

type legacyRuntimeCleanup struct {
	root              *Root
	projectPath       string
	runtimeRoot       string
	quarantine        string
	moved             []string
	backups           map[string]legacyRuntimeBackup
	quarantineRemoved bool
	deletionDurable   bool
	finalized         bool
	rolledBack        bool
}

type legacyRuntimeBackup struct {
	data    []byte
	target  string
	mode    fs.FileMode
	symlink bool
}

func (r *Root) cleanupLegacyRuntimeFiles(projectPath, runtimeRoot string, names []string) (*legacyRuntimeCleanup, error) {
	cleanup := &legacyRuntimeCleanup{root: r, projectPath: projectPath, runtimeRoot: runtimeRoot, backups: make(map[string]legacyRuntimeBackup)}
	if len(names) == 0 {
		cleanup.finalized = true
		return cleanup, nil
	}
	quarantine, err := os.MkdirTemp(runtimeRoot, ".legacy-quarantine-")
	if err != nil {
		return cleanup, fmt.Errorf("create legacy quarantine: %w", err)
	}
	cleanup.quarantine = quarantine
	for _, name := range names {
		path := filepath.Join(projectPath, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return cleanup, cleanup.fail(fmt.Errorf("inspect legacy runtime file %s: %w", name, err))
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		backup := legacyRuntimeBackup{mode: info.Mode().Perm(), symlink: info.Mode()&os.ModeSymlink != 0}
		if backup.symlink {
			backup.target, err = os.Readlink(path)
		} else {
			backup.data, err = os.ReadFile(path)
		}
		if err != nil {
			return cleanup, cleanup.fail(fmt.Errorf("backup legacy runtime file %s: %w", name, err))
		}
		cleanup.backups[name] = backup
		if err := r.moveLegacyRuntime(path, filepath.Join(cleanup.quarantine, name)); err != nil {
			return cleanup, cleanup.fail(fmt.Errorf("move legacy runtime file %s: %w", name, err))
		}
		cleanup.moved = append(cleanup.moved, name)
	}
	if len(cleanup.moved) == 0 {
		if err := os.RemoveAll(cleanup.quarantine); err != nil {
			return cleanup, fmt.Errorf("remove empty legacy quarantine: %w", err)
		}
		cleanup.finalized = true
		return cleanup, nil
	}
	// Persist both sides of every rename before deleting the quarantine. A
	// failure here is still reversible because all moved entries remain in it.
	if err := r.syncRuntimeDirectory(cleanup.quarantine); err != nil {
		return cleanup, cleanup.fail(err)
	}
	if err := r.syncRuntimeDirectory(projectPath); err != nil {
		return cleanup, cleanup.fail(err)
	}
	if err := r.syncRuntimeDirectory(runtimeRoot); err != nil {
		return cleanup, cleanup.fail(err)
	}
	if err := r.removeRuntimePath(cleanup.quarantine); err != nil {
		return cleanup, cleanup.fail(fmt.Errorf("remove legacy quarantine: %w", err))
	}
	cleanup.quarantineRemoved = true
	if err := r.syncRuntimeDirectory(runtimeRoot); err != nil {
		return cleanup, cleanup.fail(err)
	}
	cleanup.deletionDurable = true
	cleanup.finalized = true
	return cleanup, nil
}

func (c *legacyRuntimeCleanup) fail(err error) error {
	if rollbackErr := c.rollback(); rollbackErr != nil {
		return errors.Join(err, rollbackErr)
	}
	return err
}

func (c *legacyRuntimeCleanup) rollback() error {
	if c == nil || c.rolledBack {
		return nil
	}
	if c.finalized {
		for _, name := range legacyRuntimeNames {
			backup, ok := c.backups[name]
			if !ok {
				continue
			}
			destination := filepath.Join(c.projectPath, name)
			if _, err := os.Lstat(destination); err == nil {
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect legacy runtime destination %s: %w", name, err)
			}
			if backup.symlink {
				if err := os.Symlink(backup.target, destination); err != nil {
					return fmt.Errorf("restore legacy runtime symlink %s: %w", name, err)
				}
			} else if err := writeAtomic(c.projectPath, name, backup.data, backup.mode); err != nil {
				return fmt.Errorf("restore legacy runtime file %s: %w", name, err)
			}
		}
		if err := c.root.syncRuntimeDirectory(c.projectPath); err != nil {
			return err
		}
		c.rolledBack = true
		return nil
	}
	var rollbackErrs []error
	for index := len(c.moved) - 1; index >= 0; index-- {
		name := c.moved[index]
		source := filepath.Join(c.quarantine, name)
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			if err := c.restoreBackup(name); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
			continue
		} else if err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("inspect quarantined legacy runtime file %s: %w", name, err))
			continue
		}
		destination := filepath.Join(c.projectPath, name)
		if _, err := os.Lstat(destination); err == nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore legacy runtime file %s: destination already exists", name))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("inspect legacy runtime destination %s: %w", name, err))
			continue
		}
		if err := c.root.moveLegacyRuntime(source, destination); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore legacy runtime file %s: %w", name, err))
		}
	}
	if len(rollbackErrs) > 0 {
		return errors.Join(rollbackErrs...)
	}
	if c.quarantine != "" {
		if err := c.root.removeRuntimePath(c.quarantine); err != nil {
			return fmt.Errorf("remove legacy quarantine after rollback: %w", err)
		}
		c.quarantineRemoved = true
	}
	if err := c.root.syncRuntimeDirectory(c.projectPath); err != nil {
		return err
	}
	if err := c.root.syncRuntimeDirectory(c.runtimeRoot); err != nil {
		return err
	}
	c.rolledBack = true
	return nil
}

func (c *legacyRuntimeCleanup) restoreBackup(name string) error {
	backup, ok := c.backups[name]
	if !ok {
		return nil
	}
	destination := filepath.Join(c.projectPath, name)
	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy runtime destination %s: %w", name, err)
	}
	if backup.symlink {
		if err := os.Symlink(backup.target, destination); err != nil {
			return fmt.Errorf("restore legacy runtime symlink %s: %w", name, err)
		}
	} else if err := writeAtomic(c.projectPath, name, backup.data, backup.mode); err != nil {
		return fmt.Errorf("restore legacy runtime file %s: %w", name, err)
	}
	return nil
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
	hooks      runtimeHooks
}

// runtimeHooks are intentionally private test seams for exercising crash and
// filesystem-failure state transitions without relying on host-specific faults.
// Production Roots leave all hooks nil and use the operating-system calls.
type runtimeHooks struct {
	syncDirectory func(string) error
	moveLegacy    func(string, string) error
	removeAll     func(string) error
	writeMetadata func(string, Metadata) error
	operation     func(string)
}

// SetMetadataWriteHookForTest injects a metadata publication fault in package
// tests; production callers never set this hook.
func (r *Root) SetMetadataWriteHookForTest(hook func(string, Metadata) error) {
	r.hooks.writeMetadata = hook
}

func (r *Root) syncRuntimeDirectory(directory string) error {
	r.recordRuntimeOperation("sync:" + directory)
	if r.hooks.syncDirectory != nil {
		return r.hooks.syncDirectory(directory)
	}
	return syncDirectory(directory)
}

func (r *Root) moveLegacyRuntime(source, destination string) error {
	if r.hooks.moveLegacy != nil {
		return r.hooks.moveLegacy(source, destination)
	}
	return os.Rename(source, destination)
}

func (r *Root) removeRuntimePath(path string) error {
	if r.hooks.removeAll != nil {
		return r.hooks.removeAll(path)
	}
	return os.RemoveAll(path)
}

func (r *Root) recordRuntimeOperation(operation string) {
	if r.hooks.operation != nil {
		r.hooks.operation(operation)
	}
}

func (r *Root) switchRuntimePointer(runtimeRoot, current, target string) error {
	return switchRuntimePointerWithOps(runtimeRoot, current, target, r.syncRuntimeDirectory, r.recordRuntimeOperation)
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
	root := &Root{base: filepath.Clean(absolute)}
	if err := root.cleanupAbandonedRuntimeCandidatesAtStartup(); err != nil {
		return nil, fmt.Errorf("clean abandoned runtime candidates: %w", err)
	}
	return root, nil
}

// cleanupAbandonedRuntimeCandidatesAtStartup is called once during
// Provisioner root initialization. It only inspects direct project
// directories and removes entries named .candidate-* below each project's
// .manager-runtime directory; current and committed generations are kept.
func (r *Root) cleanupAbandonedRuntimeCandidatesAtStartup() error {
	entries, err := os.ReadDir(r.base)
	if err != nil {
		return fmt.Errorf("list project root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !slugPattern.MatchString(entry.Name()) {
			continue
		}
		if err := r.CleanupAbandonedRuntimeCandidates(entry.Name()); err != nil {
			return fmt.Errorf("clean project %s: %w", entry.Name(), err)
		}
	}
	return nil
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

// UpdateMetadataWithRollback keeps a runtime rollback closure live until the
// metadata publication succeeds. Mutation errors are persisted deliberately so
// typed idempotent failure outcomes can be replayed without Docker work.
func (r *Root) UpdateMetadataWithRollback(slug string, mutate func(*Metadata) error, rollback func() error) (Metadata, error) {
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
	mutationErr := mutate(&metadata)
	if errors.Is(mutationErr, contracts.ErrStaleConfigRevision) {
		return Metadata{}, mutationErr
	}
	if writeErr := r.writeMetadata(slug, metadata); writeErr != nil {
		if rollback != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return Metadata{}, errors.Join(writeErr, rollbackErr)
			}
		}
		return Metadata{}, writeErr
	}
	return metadata, mutationErr
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
	if r.hooks.writeMetadata != nil {
		return r.hooks.writeMetadata(slug, metadata)
	}
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
