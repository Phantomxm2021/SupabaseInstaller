package projectfs

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"supabase-manager/internal/contracts"
)

const (
	maxFunctionArchiveBytes   = 20 << 20
	maxFunctionExtractedBytes = 100 << 20
	maxFunctionFiles          = 500
	maxFunctionFileBytes      = 20 << 20
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// FunctionReleaseStage is an extracted, unactivated release. Its path remains
// private to Provisioner callers and is never returned by an HTTP handler.
type FunctionReleaseStage struct {
	SHA256      string
	OperationID string
	Name        string
	path        string
}

// FunctionActivation records the safe release metadata after an activation.
type FunctionActivation struct {
	Current  *contracts.FunctionRelease
	Previous *contracts.FunctionRelease
}

type managedFunctionState struct {
	Current  *contracts.FunctionRelease `json:"current,omitempty"`
	Previous *contracts.FunctionRelease `json:"previous,omitempty"`
}

// ListFunctions reads only Manager-owned state and returns no filesystem paths
// or source content. Unmanaged function directories are deliberately omitted.
func (r *Root) ListFunctions(slug string) ([]contracts.FunctionSummary, error) {
	project, err := r.ProjectPath(slug)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(project, "volumes", "functions", ".manager")
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return []contracts.FunctionSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list managed functions: %w", err)
	}
	result := make([]contracts.FunctionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || contracts.ValidateFunctionName(entry.Name()) != nil {
			continue
		}
		state, err := readManagedFunctionState(filepath.Join(base, entry.Name(), "releases"))
		if err != nil {
			return nil, err
		}
		if state.Current != nil {
			result = append(result, contracts.FunctionSummary{Name: entry.Name(), Current: state.Current, Previous: state.Previous})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// FunctionCurrentPath returns the current managed pointer for a function.
func (r *Root) FunctionCurrentPath(slug, name string) (string, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return "", err
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return "", err
	}
	current := filepath.Join(project, "volumes", "functions", name)
	if _, err := os.Lstat(current); err != nil {
		return "", err
	}
	return current, nil
}

// StageFunctionRelease accepts a complete ZIP stream and extracts it only to
// a project-contained staging directory. It never creates the live pointer.
func (r *Root) StageFunctionRelease(slug, name, operationID string, archive io.Reader) (FunctionReleaseStage, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return FunctionReleaseStage{}, err
	}
	if !operationIDPattern.MatchString(operationID) {
		return FunctionReleaseStage{}, fmt.Errorf("invalid function operation id")
	}
	contents, err := io.ReadAll(io.LimitReader(archive, maxFunctionArchiveBytes+1))
	if err != nil {
		return FunctionReleaseStage{}, fmt.Errorf("read function archive: %w", err)
	}
	if len(contents) > maxFunctionArchiveBytes {
		return FunctionReleaseStage{}, fmt.Errorf("function archive exceeds 20 MiB")
	}
	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return FunctionReleaseStage{}, fmt.Errorf("open function ZIP: %w", err)
	}
	if len(reader.File) > maxFunctionFiles {
		return FunctionReleaseStage{}, fmt.Errorf("function archive contains too many files")
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return FunctionReleaseStage{}, err
	}
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	base := filepath.Join(project, "volumes", "functions", ".manager", name, "staging")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return FunctionReleaseStage{}, fmt.Errorf("create function staging root: %w", err)
	}
	stage, err := os.MkdirTemp(base, operationID+"-")
	if err != nil {
		return FunctionReleaseStage{}, fmt.Errorf("create function staging directory: %w", err)
	}
	fail := func(cause error) (FunctionReleaseStage, error) {
		_ = os.RemoveAll(stage)
		return FunctionReleaseStage{}, cause
	}
	var extracted int64
	hasIndex := false
	stripPrefix := functionArchivePrefix(reader.File, name)
	paths := make(map[string]struct{}, len(reader.File))
	for _, item := range reader.File {
		clean, isDirectory, err := safeFunctionArchivePath(item)
		if err != nil {
			return fail(err)
		}
		if isFunctionArchiveMetadata(clean) {
			continue
		}
		if stripPrefix != "" {
			prefixRoot := strings.TrimSuffix(stripPrefix, "/")
			if isDirectory && strings.HasPrefix(prefixRoot, clean+"/") {
				continue
			}
			if clean == name {
				if isDirectory {
					continue
				}
				return fail(fmt.Errorf("function archive contains an invalid enclosing directory"))
			}
			if !strings.HasPrefix(clean, stripPrefix) {
				return fail(fmt.Errorf("function archive contains an invalid enclosing directory"))
			}
			clean = strings.TrimPrefix(clean, stripPrefix)
			if clean == "" {
				if isDirectory {
					continue
				}
				return fail(fmt.Errorf("function archive contains an invalid enclosing directory"))
			}
		}
		if isDirectory {
			if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(clean)), 0o700); err != nil {
				return fail(fmt.Errorf("create function directory: %w", err))
			}
			continue
		}
		if _, found := paths[clean]; found {
			return fail(fmt.Errorf("function archive contains duplicate path"))
		}
		paths[clean] = struct{}{}
		if item.UncompressedSize64 > maxFunctionFileBytes {
			return fail(fmt.Errorf("function archive file exceeds 20 MiB"))
		}
		if !isDirectory && clean == "index.ts" {
			hasIndex = true
		}
		output := filepath.Join(stage, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			return fail(fmt.Errorf("create function parent directory: %w", err))
		}
		source, err := item.Open()
		if err != nil {
			return fail(fmt.Errorf("open function archive entry: %w", err))
		}
		var copied int64
		destination, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			copied, err = io.Copy(destination, io.LimitReader(source, maxFunctionFileBytes+1))
			if err == nil && copied > maxFunctionFileBytes {
				err = fmt.Errorf("function archive file exceeds 20 MiB")
			}
			closeErr := destination.Close()
			if err == nil {
				err = closeErr
			}
		}
		_ = source.Close()
		if err != nil {
			return fail(fmt.Errorf("extract function archive entry: %w", err))
		}
		extracted += copied
		if extracted > maxFunctionExtractedBytes {
			return fail(fmt.Errorf("function archive exceeds 100 MiB when extracted"))
		}
	}
	if !hasIndex {
		return fail(fmt.Errorf("function archive requires index.ts at root or under the requested function directory"))
	}
	if err := syncDirectory(stage); err != nil {
		return fail(err)
	}
	hash := sha256.Sum256(contents)
	return FunctionReleaseStage{SHA256: hex.EncodeToString(hash[:]), OperationID: operationID, Name: name, path: stage}, nil
}

// ActivateFunctionRelease publishes a complete stage through an atomically
// replaced relative symlink. The target and link live on the same project
// filesystem, so readers observe either the former pointer or the new one.
func (r *Root) ActivateFunctionRelease(slug, name string, stage FunctionReleaseStage) (FunctionActivation, error) {
	if err := contracts.ValidateFunctionName(name); err != nil || stage.Name != name || stage.path == "" {
		return FunctionActivation{}, fmt.Errorf("invalid function release stage")
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return FunctionActivation{}, err
	}
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	functionsRoot := filepath.Join(project, "volumes", "functions")
	releases := filepath.Join(functionsRoot, ".manager", name, "releases")
	if err := os.MkdirAll(releases, 0o700); err != nil {
		return FunctionActivation{}, fmt.Errorf("create function releases directory: %w", err)
	}
	release := filepath.Join(releases, stage.SHA256)
	if _, err := os.Lstat(release); err == nil {
		return FunctionActivation{}, fmt.Errorf("function release already exists")
	} else if !os.IsNotExist(err) {
		return FunctionActivation{}, err
	}
	if err := os.Rename(stage.path, release); err != nil {
		return FunctionActivation{}, fmt.Errorf("publish function release: %w", err)
	}
	current := filepath.Join(functionsRoot, name)
	if info, err := os.Lstat(current); err == nil && (info.Mode()&os.ModeSymlink == 0 || !isManagedFunctionPointer(current, releases)) {
		_ = os.RemoveAll(release)
		return FunctionActivation{}, fmt.Errorf("function is not managed by Manager")
	} else if err != nil && !os.IsNotExist(err) {
		return FunctionActivation{}, err
	}
	state, err := readManagedFunctionState(releases)
	if err != nil {
		return FunctionActivation{}, err
	}
	if err := switchFunctionPointer(functionsRoot, name, stage.OperationID, stage.SHA256); err != nil {
		return FunctionActivation{}, err
	}
	if err := syncDirectory(functionsRoot); err != nil {
		return FunctionActivation{}, err
	}
	currentRelease := &contracts.FunctionRelease{SHA256: stage.SHA256, OperationID: stage.OperationID, DeployedAt: time.Now().UTC()}
	if err := writeManagedFunctionState(releases, managedFunctionState{Current: currentRelease, Previous: state.Current}); err != nil {
		return FunctionActivation{}, err
	}
	// Keep only the two releases exposed by the API. Cleanup is best-effort so
	// a stale artifact can never make a successful activation fail.
	_ = pruneFunctionReleases(releases, currentRelease.SHA256, func() string {
		if state.Current == nil {
			return ""
		}
		return state.Current.SHA256
	}())
	return FunctionActivation{Current: currentRelease, Previous: state.Current}, nil
}

// DeleteFunction removes only Manager-owned releases and their current
// pointer. An unmanaged directory named like a function is never touched.
func (r *Root) DeleteFunction(slug, name string) (FunctionActivation, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return FunctionActivation{}, err
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return FunctionActivation{}, err
	}
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	functionsRoot := filepath.Join(project, "volumes", "functions")
	managerDir := filepath.Join(functionsRoot, ".manager", name)
	releases := filepath.Join(managerDir, "releases")
	state, err := readManagedFunctionState(releases)
	if err != nil {
		return FunctionActivation{}, err
	}
	if state.Current == nil {
		return FunctionActivation{}, fmt.Errorf("function is not managed")
	}
	current := filepath.Join(functionsRoot, name)
	if info, err := os.Lstat(current); err == nil {
		if info.Mode()&os.ModeSymlink == 0 || !isManagedFunctionPointer(current, managerDir) {
			return FunctionActivation{}, fmt.Errorf("function is not managed by Manager")
		}
		_ = os.Remove(current)
	} else if !os.IsNotExist(err) {
		return FunctionActivation{}, err
	}
	if err := os.RemoveAll(managerDir); err != nil {
		return FunctionActivation{}, fmt.Errorf("delete function releases: %w", err)
	}
	_ = syncDirectory(filepath.Join(functionsRoot, ".manager"))
	return FunctionActivation{Current: state.Current, Previous: state.Previous}, nil
}

func pruneFunctionReleases(releases, current, previous string) error {
	entries, err := os.ReadDir(releases)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current || entry.Name() == previous {
			continue
		}
		if len(entry.Name()) != 64 {
			continue
		}
		if err := os.RemoveAll(filepath.Join(releases, entry.Name())); err != nil {
			return err
		}
	}
	return syncDirectory(releases)
}

func isManagedFunctionPointer(pointer, releases string) bool {
	target, err := os.Readlink(pointer)
	if err != nil || filepath.IsAbs(target) {
		return false
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(pointer), target))
	releaseRoot := filepath.Clean(releases)
	rel, err := filepath.Rel(releaseRoot, resolved)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RollbackFunctionRelease makes the immediately preceding successful release
// current. It is intentionally unavailable when a function has only one
// retained release.
func (r *Root) RollbackFunctionRelease(slug, name, operationID string) (FunctionActivation, error) {
	if err := contracts.ValidateFunctionName(name); err != nil || !operationIDPattern.MatchString(operationID) {
		return FunctionActivation{}, fmt.Errorf("invalid function rollback request")
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return FunctionActivation{}, err
	}
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	functionsRoot := filepath.Join(project, "volumes", "functions")
	releases := filepath.Join(functionsRoot, ".manager", name, "releases")
	state, err := readManagedFunctionState(releases)
	if err != nil {
		return FunctionActivation{}, err
	}
	if state.Current == nil || state.Previous == nil {
		return FunctionActivation{}, fmt.Errorf("no previous function release")
	}
	if _, err := os.Stat(filepath.Join(releases, state.Previous.SHA256, "index.ts")); err != nil {
		return FunctionActivation{}, fmt.Errorf("previous function release is unavailable: %w", err)
	}
	if err := switchFunctionPointer(functionsRoot, name, operationID, state.Previous.SHA256); err != nil {
		return FunctionActivation{}, err
	}
	if err := syncDirectory(functionsRoot); err != nil {
		return FunctionActivation{}, err
	}
	if err := writeManagedFunctionState(releases, managedFunctionState{Current: state.Previous, Previous: state.Current}); err != nil {
		return FunctionActivation{}, err
	}
	return FunctionActivation{Current: state.Previous, Previous: state.Current}, nil
}

// RestoreFunctionRelease compensates a failed post-activation action by
// restoring the activation's previous pointer and durable state.
func (r *Root) RestoreFunctionRelease(slug, name string, activation FunctionActivation) error {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return err
	}
	if activation.Previous == nil {
		return fmt.Errorf("no previous function release to restore")
	}
	project, err := r.ProjectPath(slug)
	if err != nil {
		return err
	}
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	functionsRoot := filepath.Join(project, "volumes", "functions")
	releases := filepath.Join(functionsRoot, ".manager", name, "releases")
	if err := switchFunctionPointer(functionsRoot, name, "restore", activation.Previous.SHA256); err != nil {
		return err
	}
	if err := syncDirectory(functionsRoot); err != nil {
		return err
	}
	return writeManagedFunctionState(releases, managedFunctionState{Current: activation.Previous, Previous: activation.Current})
}

func switchFunctionPointer(functionsRoot, name, operationID, sha string) error {
	temporary := filepath.Join(functionsRoot, "."+name+".next-"+operationID)
	_ = os.Remove(temporary)
	target := filepath.ToSlash(filepath.Join(".manager", name, "releases", sha))
	if err := os.Symlink(target, temporary); err != nil {
		return fmt.Errorf("create function pointer: %w", err)
	}
	if err := os.Rename(temporary, filepath.Join(functionsRoot, name)); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("activate function pointer: %w", err)
	}
	return nil
}

func readManagedFunctionState(releases string) (managedFunctionState, error) {
	data, err := os.ReadFile(filepath.Join(releases, "state.json"))
	if os.IsNotExist(err) {
		return managedFunctionState{}, nil
	}
	if err != nil {
		return managedFunctionState{}, fmt.Errorf("read function release state: %w", err)
	}
	var state managedFunctionState
	if err := json.Unmarshal(data, &state); err != nil {
		return managedFunctionState{}, fmt.Errorf("parse function release state: %w", err)
	}
	return state, nil
}

func writeManagedFunctionState(releases string, state managedFunctionState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode function release state: %w", err)
	}
	if err := writeAtomic(releases, "state.json", data, 0o600); err != nil {
		return err
	}
	return syncDirectory(releases)
}

func safeFunctionArchivePath(item *zip.File) (string, bool, error) {
	name := item.Name
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", false, fmt.Errorf("function archive contains unsafe path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", false, fmt.Errorf("function archive contains unsafe path")
	}
	mode := item.Mode()
	if mode&fs.ModeSymlink != 0 || mode&fs.ModeType != 0 && !mode.IsRegular() && !mode.IsDir() {
		return "", false, fmt.Errorf("function archive contains unsupported file type")
	}
	return clean, item.FileInfo().IsDir(), nil
}

func isFunctionArchiveMetadata(clean string) bool {
	if clean == "__MACOSX" || strings.HasPrefix(clean, "__MACOSX/") {
		return true
	}
	base := path.Base(clean)
	return base == ".DS_Store" || strings.HasPrefix(base, "._")
}

// functionArchivePrefix returns a trusted directory prefix to remove before
// activation. It accepts the layouts users commonly produce when zipping a
// Supabase function: the function folder itself and the canonical
// supabase/functions/<name> path. Ancestor directory entries are tolerated,
// but files outside the selected prefix invalidate that layout.
func functionArchivePrefix(files []*zip.File, name string) string {
	candidates := []string{"", name + "/", "supabase/functions/" + name + "/"}
	knownCandidates := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		knownCandidates[candidate] = struct{}{}
	}
	// A project-level ZIP often adds an outer repository directory. Discover
	// that wrapper from a root index.ts whose immediate parent is the requested
	// function name, then apply the same containment checks below.
	for _, item := range files {
		clean, isDirectory, err := safeFunctionArchivePath(item)
		if err != nil || isDirectory || isFunctionArchiveMetadata(clean) || !strings.HasSuffix(clean, "/index.ts") {
			continue
		}
		prefix := strings.TrimSuffix(clean, "index.ts")
		if path.Base(strings.TrimSuffix(prefix, "/")) != name {
			continue
		}
		if _, exists := knownCandidates[prefix]; !exists {
			knownCandidates[prefix] = struct{}{}
			candidates = append(candidates, prefix)
		}
	}
	for _, prefix := range candidates {
		hasIndex := false
		valid := true
		root := strings.TrimSuffix(prefix, "/")
		for _, item := range files {
			clean, isDirectory, err := safeFunctionArchivePath(item)
			if err != nil || isFunctionArchiveMetadata(clean) {
				continue
			}
			if prefix == "" {
				if !isDirectory && clean == "index.ts" {
					hasIndex = true
				}
				continue
			}
			if strings.HasPrefix(clean, prefix) {
				relative := strings.TrimPrefix(clean, prefix)
				if relative == "" {
					if !isDirectory {
						valid = false
					}
					continue
				}
				if !isDirectory && relative == "index.ts" {
					hasIndex = true
				}
				continue
			}
			// ZIP writers often include explicit entries for the prefix's
			// ancestors. They are safe to ignore during normalization.
			if isDirectory && root != "" && strings.HasPrefix(root, clean+"/") {
				continue
			}
			valid = false
			break
		}
		if valid && hasIndex {
			return prefix
		}
	}
	return ""
}
