package contracts

import "testing"

func TestManagedTLSPathsUseBaseDomainLabel(t *testing.T) {
	got, err := ManagedTLSPaths("cloudflare-origin", "https://beegame.studio")
	if err != nil {
		t.Fatal(err)
	}
	if got.CertificateName != "cloudflare-origin" {
		t.Fatalf("certificate name = %q", got.CertificateName)
	}
	if got.CertificateFile != "/etc/nginx/ssl/cloudflare-origin-beegame.pem" {
		t.Fatalf("certificate file = %q", got.CertificateFile)
	}
	if got.PrivateKeyFile != "/etc/nginx/ssl/cloudflare-origin-beegame.key" {
		t.Fatalf("private key file = %q", got.PrivateKeyFile)
	}
}

func TestManagedTLSPathsRejectUnsafeName(t *testing.T) {
	if _, err := ManagedTLSPaths("../origin", "https://beegame.studio"); err == nil {
		t.Fatal("ManagedTLSPaths accepted path traversal")
	}
}

func TestManagedTLSPathsRejectNonBaseDomainURL(t *testing.T) {
	if _, err := ManagedTLSPaths("cloudflare-origin", "https://localhost"); err == nil {
		t.Fatal("ManagedTLSPaths accepted a hostname without a base domain")
	}
}
