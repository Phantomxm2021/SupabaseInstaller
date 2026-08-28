package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenUnixReplacesOnlyExistingSocket(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "npa-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "agent.sock")
	first, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("first ListenUnix() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("second ListenUnix() error = %v", err)
	}
	defer second.Close()

	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	_ = connection.Close()
}
