package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"supabase-manager/internal/contracts"
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
	var gotRoute Route
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotAuthorization, gotPath = request.Header.Get("Authorization"), request.URL.Path
		if request.URL.Path == "/v1/sites/apply" {
			if err := json.NewDecoder(request.Body).Decode(&gotRoute); err != nil {
				t.Errorf("decode route: %v", err)
			}
		}
		if request.URL.Path == "/v1/certificates/stage" {
			var input contracts.StageManagedTLSRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode certificate request: %v", err)
			}
			if string(input.PrivateKeyPEM) != "private-key" {
				t.Errorf("private key was not sent to host agent")
			}
			_ = json.NewEncoder(response).Encode(contracts.StageManagedTLSResponse{ManagedTLSConfig: contracts.ManagedTLSConfig{CertificateName: input.CertificateName, CertificateFile: "/etc/nginx/ssl/cloudflare-origin-example.pem", PrivateKeyFile: "/etc/nginx/ssl/cloudflare-origin-example.key"}, Created: true})
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })

	client := NewManagedClient(socket, "agent-token")
	if err := client.Apply(context.Background(), Route{Slug: "bee", Domain: "bee.example.com", APIPort: 18001, StudioPort: 18002, StudioEnabled: true, StudioUsername: "operator", StudioPassword: "studio-password"}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got, want := gotAuthorization, "Bearer agent-token"; got != want {
		t.Fatalf("authorization = %q, want %q", got, want)
	}
	if got, want := gotPath, "/v1/sites/apply"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if got, want := gotRoute.StudioUsername, "operator"; got != want {
		t.Fatalf("Studio username = %q, want %q", got, want)
	}
	if got, want := gotRoute.StudioPassword, "studio-password"; got != want {
		t.Fatalf("Studio password was not sent across private socket")
	}

	if err := client.Remove(context.Background(), "bee"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got, want := gotPath, "/v1/sites/remove"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	staged, err := client.StageCertificate(context.Background(), contracts.StageManagedTLSRequest{CertificateName: "cloudflare-origin", BaseDomain: "example.com", CertificatePEM: []byte("certificate"), PrivateKeyPEM: []byte("private-key")})
	if err != nil {
		t.Fatalf("StageCertificate() error = %v", err)
	}
	if got, want := staged.CertificateFile, "/etc/nginx/ssl/cloudflare-origin-example.pem"; got != want {
		t.Fatalf("certificate file = %q, want %q", got, want)
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
	if _, err := client.StageCertificate(context.Background(), contracts.StageManagedTLSRequest{}); err == nil {
		t.Fatal("StageCertificate() succeeded with disabled managed proxy")
	}
}
