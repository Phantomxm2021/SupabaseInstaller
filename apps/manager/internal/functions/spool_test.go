package functions

import (
	"bytes"
	"os"
	"testing"
)

func TestSpoolStoresOperationArchiveWithRestrictedPermissions(t *testing.T) {
	spool, err := NewSpool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path, hash, err := spool.Store("operation-1", bytes.NewBufferString("zip-body"))
	if err != nil || hash == "" {
		t.Fatalf("Store() = %q, %q, %v", path, hash, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("spool permissions = %v, %v", info.Mode(), err)
	}
	if err := spool.Remove("operation-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("spool file remains: %v", err)
	}
}
