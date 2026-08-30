package functions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const maxArchiveBytes = 20 << 20

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Spool struct{ directory string }

func NewSpool(directory string) (*Spool, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("function upload spool directory must be absolute")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create function upload spool: %w", err)
	}
	return &Spool{directory: filepath.Clean(directory)}, nil
}

func (s *Spool) Store(operationID string, source io.Reader) (string, string, error) {
	if s == nil || !operationIDPattern.MatchString(operationID) || source == nil {
		return "", "", fmt.Errorf("invalid function upload")
	}
	temporary, err := os.CreateTemp(s.directory, "."+operationID+"-")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", "", err
	}
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, maxArchiveBytes+1))
	if err != nil {
		_ = temporary.Close()
		return "", "", err
	}
	if count > maxArchiveBytes {
		_ = temporary.Close()
		return "", "", fmt.Errorf("function archive exceeds 20 MiB")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", "", err
	}
	if err := temporary.Close(); err != nil {
		return "", "", err
	}
	path := filepath.Join(s.directory, operationID+".zip")
	if err := os.Rename(temporary.Name(), path); err != nil {
		return "", "", err
	}
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Spool) Open(operationID string) (*os.File, error) {
	if s == nil || !operationIDPattern.MatchString(operationID) {
		return nil, fmt.Errorf("invalid function upload")
	}
	return os.Open(filepath.Join(s.directory, operationID+".zip"))
}
func (s *Spool) Remove(operationID string) error {
	if s == nil || !operationIDPattern.MatchString(operationID) {
		return fmt.Errorf("invalid function upload")
	}
	err := os.Remove(filepath.Join(s.directory, operationID+".zip"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
