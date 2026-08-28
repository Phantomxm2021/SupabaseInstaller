package proxy

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestClientCallsTypedApplyAndRemoveOverUnixSocket(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "npc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var gotAuthorization, gotPath string
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotAuthorization, gotPath = request.Header.Get("Authorization"), request.URL.Path
		response.WriteHeader(http.StatusNoContent)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })

	client := NewManagedClient(socket, "agent-token")
	if err := client.Apply(context.Background(), Route{Slug: "bee", Domain: "bee.example.com", APIPort: 18001, StudioPort: 18002, StudioEnabled: true}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got, want := gotAuthorization, "Bearer agent-token"; got != want {
		t.Fatalf("authorization = %q, want %q", got, want)
	}
	if got, want := gotPath, "/v1/sites/apply"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	if err := client.Remove(context.Background(), "bee"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got, want := gotPath, "/v1/sites/remove"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestDisabledClientDoesNothing(t *testing.T) {
	client := DisabledClient{}
	if err := client.Apply(context.Background(), Route{}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := client.Remove(context.Background(), "bee"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}
