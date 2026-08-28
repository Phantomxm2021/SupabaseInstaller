package site

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	availableNamePattern  = regexp.MustCompile(`^supabase-manager-[a-z0-9][a-z0-9-]{0,62}\.conf$`)
	credentialNamePattern = regexp.MustCompile(`^supabase-manager-[a-z0-9][a-z0-9-]{0,62}\.htpasswd$`)
)

// CommandRunner is the only privileged action Store needs after a filesystem
// mutation. Implementations must run nginx -t and a reload respectively.
type CommandRunner interface {
	Test(context.Context) error
	Reload(context.Context) error
}

// Store owns only one managed site in the configured sites-available and
// sites-enabled directories. It snapshots the old state before every change
// and restores it if nginx validation or reload fails.
type Store struct {
	availableDirectory string
	enabledDirectory   string
	authDirectory      string
	runner             CommandRunner
}

func NewStore(availableDirectory, enabledDirectory, authDirectory string, runner CommandRunner) Store {
	return Store{
		availableDirectory: availableDirectory,
		enabledDirectory:   enabledDirectory,
		authDirectory:      authDirectory,
		runner:             runner,
	}
}

func (s Store) Apply(ctx context.Context, rendered RenderedSite) error {
	if err := s.validate(rendered.AvailableName, rendered.AuthFileName); err != nil {
		return err
	}
	if rendered.AuthDirectory != "" && filepath.Clean(rendered.AuthDirectory) != filepath.Clean(s.authDirectory) {
		return fmt.Errorf("credential directory does not match Agent configuration")
	}

	previous, err := s.snapshot(rendered.AvailableName, rendered.AuthFileName)
	if err != nil {
		return err
	}
	if rendered.AuthFileName != "" {
		if rendered.AuthContents == "" {
			if err := removeIfPresent(s.authPath(rendered.AuthFileName)); err != nil {
				return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("remove Studio credentials: %w", err))
			}
		} else if err := writeAtomic(s.authDirectory, rendered.AuthFileName, []byte(rendered.AuthContents), 0o644); err != nil {
			return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("write Studio credentials: %w", err))
		}
	}
	if err := writeAtomic(s.availableDirectory, rendered.AvailableName, []byte(rendered.Contents), 0o644); err != nil {
		return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("write available site: %w", err))
	}
	if err := s.replaceEnabledLink(rendered.AvailableName, s.availablePath(rendered.AvailableName)); err != nil {
		return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("activate site: %w", err))
	}
	if err := s.runner.Test(ctx); err != nil {
		return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("validate nginx configuration: %w", err))
	}
	if err := s.runner.Reload(ctx); err != nil {
		return s.restoreAfterFailure(ctx, rendered.AvailableName, rendered.AuthFileName, previous, fmt.Errorf("reload nginx: %w", err))
	}
	return nil
}

func (s Store) Remove(ctx context.Context, availableName string) error {
	authFileName, err := ManagedCredentialNameFromSite(availableName)
	if err != nil {
		return err
	}
	if err := s.validate(availableName, authFileName); err != nil {
		return err
	}

	previous, err := s.snapshot(availableName, authFileName)
	if err != nil {
		return err
	}
	if err := removeIfPresent(s.enabledPath(availableName)); err != nil {
		return s.restoreAfterFailure(ctx, availableName, authFileName, previous, fmt.Errorf("deactivate site: %w", err))
	}
	if err := removeIfPresent(s.availablePath(availableName)); err != nil {
		return s.restoreAfterFailure(ctx, availableName, authFileName, previous, fmt.Errorf("remove available site: %w", err))
	}
	if err := removeIfPresent(s.authPath(authFileName)); err != nil {
		return s.restoreAfterFailure(ctx, availableName, authFileName, previous, fmt.Errorf("remove Studio credentials: %w", err))
	}
	if err := s.runner.Test(ctx); err != nil {
		return s.restoreAfterFailure(ctx, availableName, authFileName, previous, fmt.Errorf("validate nginx configuration: %w", err))
	}
	if err := s.runner.Reload(ctx); err != nil {
		return s.restoreAfterFailure(ctx, availableName, authFileName, previous, fmt.Errorf("reload nginx: %w", err))
	}
	return nil
}

func (s Store) validate(availableName, authFileName string) error {
	if !availableNamePattern.MatchString(availableName) {
		return fmt.Errorf("invalid managed site name")
	}
	if authFileName != "" && !credentialNamePattern.MatchString(authFileName) {
		return fmt.Errorf("invalid managed credential name")
	}
	if s.runner == nil {
		return fmt.Errorf("nginx command runner is required")
	}
	for _, directory := range []string{s.availableDirectory, s.enabledDirectory, s.authDirectory} {
		info, err := os.Stat(directory)
		if err != nil {
			return fmt.Errorf("inspect nginx site directory %s: %w", directory, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("nginx site path %s is not a directory", directory)
		}
	}
	return nil
}

type siteSnapshot struct {
	available  fileSnapshot
	enabled    linkSnapshot
	credential fileSnapshot
}

type fileSnapshot struct {
	exists bool
	data   []byte
	mode   fs.FileMode
}

type linkSnapshot struct {
	exists bool
	target string
}

func (s Store) snapshot(availableName, authFileName string) (siteSnapshot, error) {
	available, err := snapshotFile(s.availablePath(availableName))
	if err != nil {
		return siteSnapshot{}, err
	}
	enabled, err := snapshotLink(s.enabledPath(availableName))
	if err != nil {
		return siteSnapshot{}, err
	}
	credential := fileSnapshot{}
	if authFileName != "" {
		credential, err = snapshotFile(s.authPath(authFileName))
		if err != nil {
			return siteSnapshot{}, err
		}
	}
	return siteSnapshot{available: available, enabled: enabled, credential: credential}, nil
}

func snapshotFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect available site %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("available site %s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read available site %s: %w", path, err)
	}
	return fileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func snapshotLink(path string) (linkSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return linkSnapshot{}, nil
	}
	if err != nil {
		return linkSnapshot{}, fmt.Errorf("inspect enabled site %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return linkSnapshot{}, fmt.Errorf("enabled site %s is not a symbolic link", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return linkSnapshot{}, fmt.Errorf("read enabled site %s: %w", path, err)
	}
	return linkSnapshot{exists: true, target: target}, nil
}

func (s Store) restoreAfterFailure(ctx context.Context, availableName, authFileName string, previous siteSnapshot, operationErr error) error {
	restoreErr := s.restore(availableName, authFileName, previous)
	if restoreErr == nil {
		if err := s.runner.Test(ctx); err != nil {
			restoreErr = fmt.Errorf("validate restored nginx configuration: %w", err)
		} else if err := s.runner.Reload(ctx); err != nil {
			restoreErr = fmt.Errorf("reload restored nginx configuration: %w", err)
		}
	}
	if restoreErr != nil {
		return errors.Join(operationErr, fmt.Errorf("restore previous nginx site: %w", restoreErr))
	}
	return operationErr
}

func (s Store) restore(availableName, authFileName string, previous siteSnapshot) error {
	if authFileName != "" {
		if previous.credential.exists {
			if err := writeAtomic(s.authDirectory, authFileName, previous.credential.data, previous.credential.mode); err != nil {
				return err
			}
		} else if err := removeIfPresent(s.authPath(authFileName)); err != nil {
			return err
		}
	}
	if previous.available.exists {
		if err := writeAtomic(s.availableDirectory, availableName, previous.available.data, previous.available.mode); err != nil {
			return err
		}
	} else if err := removeIfPresent(s.availablePath(availableName)); err != nil {
		return err
	}

	if previous.enabled.exists {
		return s.replaceEnabledLink(availableName, previous.enabled.target)
	}
	return removeIfPresent(s.enabledPath(availableName))
}

func (s Store) replaceEnabledLink(availableName, target string) error {
	temporary, err := os.CreateTemp(s.enabledDirectory, "."+availableName+"-*")
	if err != nil {
		return fmt.Errorf("create temporary enabled-site name: %w", err)
	}
	temporaryName := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("close temporary enabled-site name: %w", err)
	}
	if err := os.Remove(temporaryName); err != nil {
		return fmt.Errorf("remove temporary enabled-site file: %w", err)
	}
	defer os.Remove(temporaryName)
	if err := os.Symlink(target, temporaryName); err != nil {
		return fmt.Errorf("create temporary enabled-site link: %w", err)
	}
	if err := os.Rename(temporaryName, s.enabledPath(availableName)); err != nil {
		return fmt.Errorf("publish enabled-site link: %w", err)
	}
	return syncDirectory(s.enabledDirectory)
}

func (s Store) availablePath(availableName string) string {
	return filepath.Join(s.availableDirectory, availableName)
}

func (s Store) enabledPath(availableName string) string {
	return filepath.Join(s.enabledDirectory, availableName)
}

func (s Store) authPath(authFileName string) string {
	return filepath.Join(s.authDirectory, authFileName)
}

// ManagedCredentialNameFromSite derives the credential name solely from an
// already validated managed site name.
func ManagedCredentialNameFromSite(availableName string) (string, error) {
	if !availableNamePattern.MatchString(availableName) {
		return "", fmt.Errorf("invalid managed site name")
	}
	return strings.TrimSuffix(availableName, ".conf") + ".htpasswd", nil
}

func writeAtomic(directory, name string, data []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(directory, "."+name+"-*")
	if err != nil {
		return fmt.Errorf("create temporary site: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set site permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary site: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary site: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary site: %w", err)
	}
	if err := os.Rename(temporaryName, filepath.Join(directory, name)); err != nil {
		return fmt.Errorf("publish site: %w", err)
	}
	return syncDirectory(directory)
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
