package site

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestCertificateStorePublishesMatchedPair(t *testing.T) {
	directory := t.TempDir()
	certificate, privateKey := testPEMPair(t)
	store := NewCertificateStore(directory)

	result, err := store.Stage(context.Background(), CertificateInput{
		Name:           "cloudflare-origin",
		BaseDomain:     "beegame.studio",
		CertificatePEM: certificate,
		PrivateKeyPEM:  privateKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(directory, "cloudflare-origin-beegame.pem"); result.CertificateFile != want {
		t.Fatalf("certificate file = %q, want %q", result.CertificateFile, want)
	}
	if want := filepath.Join(directory, "cloudflare-origin-beegame.key"); result.PrivateKeyFile != want {
		t.Fatalf("private key file = %q, want %q", result.PrivateKeyFile, want)
	}
	assertFileMode(t, result.CertificateFile, 0o644)
	assertFileMode(t, result.PrivateKeyFile, 0o600)
}

func TestCertificateStoreRejectsMismatchedPrivateKeyWithoutWritingFiles(t *testing.T) {
	directory := t.TempDir()
	certificate, _ := testPEMPair(t)
	_, differentKey := testPEMPair(t)

	_, err := NewCertificateStore(directory).Stage(context.Background(), CertificateInput{
		Name:           "cloudflare-origin",
		BaseDomain:     "beegame.studio",
		CertificatePEM: certificate,
		PrivateKeyPEM:  differentKey,
	})
	if err == nil {
		t.Fatal("Stage accepted a mismatched private key")
	}
	if _, err := os.Stat(filepath.Join(directory, "cloudflare-origin-beegame.pem")); !os.IsNotExist(err) {
		t.Fatalf("certificate exists after failed stage: %v", err)
	}
}

func testPEMPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: new(big.Int).SetInt64(1)}, &x509.Certificate{PublicKey: key.Public(), SignatureAlgorithm: x509.ECDSAWithSHA256}, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}
