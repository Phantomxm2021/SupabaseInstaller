package projectfs

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

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
	paths := make(map[string]struct{}, len(reader.File))
	for _, item := range reader.File {
		clean, isDirectory, err := safeFunctionArchivePath(item)
		if err != nil {
			return fail(err)
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
		extracted += int64(item.UncompressedSize64)
		if extracted > maxFunctionExtractedBytes {
			return fail(fmt.Errorf("function archive exceeds 100 MiB when extracted"))
		}
		if clean == "index.ts" {
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
		destination, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = io.Copy(destination, io.LimitReader(source, maxFunctionFileBytes+1))
			closeErr := destination.Close()
			if err == nil {
				err = closeErr
			}
		}
		_ = source.Close()
		if err != nil {
			return fail(fmt.Errorf("extract function archive entry: %w", err))
		}
	}
	if !hasIndex {
		return fail(fmt.Errorf("function archive requires root index.ts"))
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
	if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink == 0 {
		_ = os.RemoveAll(release)
		return FunctionActivation{}, fmt.Errorf("function is not managed by Manager")
	} else if err != nil && !os.IsNotExist(err) {
		return FunctionActivation{}, err
	}
	temporary := filepath.Join(functionsRoot, "."+name+".next-"+stage.OperationID)
	_ = os.Remove(temporary)
	target := filepath.ToSlash(filepath.Join(".manager", name, "releases", stage.SHA256))
	if err := os.Symlink(target, temporary); err != nil {
		return FunctionActivation{}, fmt.Errorf("create function pointer: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return FunctionActivation{}, fmt.Errorf("activate function pointer: %w", err)
	}
	if err := syncDirectory(functionsRoot); err != nil {
		return FunctionActivation{}, err
	}
	return FunctionActivation{Current: &contracts.FunctionRelease{SHA256: stage.SHA256, OperationID: stage.OperationID}}, nil
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
