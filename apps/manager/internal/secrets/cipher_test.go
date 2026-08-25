package secrets

import (
	"bytes"
	"testing"
)

func TestCipherRoundTripBindsProjectAndKind(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	envelope, err := cipher.Encrypt("project-a", "postgres-password", []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := cipher.Decrypt("project-a", "postgres-password", envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(got) != "secret" {
		t.Fatalf("Decrypt() = %q, want secret", got)
	}
	if _, err := cipher.Decrypt("project-b", "postgres-password", envelope); err == nil {
		t.Fatal("Decrypt() with another project succeeded, want authentication error")
	}
}

func TestCipherUsesFreshNonce(t *testing.T) {
	cipher, _ := NewCipher(bytes.Repeat([]byte{9}, 32))
	first, _ := cipher.Encrypt("project-a", "jwt", []byte("same"))
	second, _ := cipher.Encrypt("project-a", "jwt", []byte("same"))
	if bytes.Equal(first.Nonce, second.Nonce) || bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("Encrypt() reused nonce or ciphertext")
	}
}

func TestNewCipherRequiresExactly256BitKey(t *testing.T) {
	if _, err := NewCipher(make([]byte, 31)); err == nil {
		t.Fatal("NewCipher() accepted a non-256-bit key")
	}
}
