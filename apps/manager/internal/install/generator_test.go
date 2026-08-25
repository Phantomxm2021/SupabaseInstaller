package install

import (
	"crypto/rand"
	"testing"
	"time"
)

func TestCryptoGeneratorMatchesOfficialSecretLengthRequirements(t *testing.T) {
	generated, err := (CryptoGenerator{Random: rand.Reader, Now: func() time.Time { return time.Unix(1_800_000_000, 0) }}).Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(generated.JWTSecret) < 32 {
		t.Fatalf("JWT secret length = %d, want at least 32", len(generated.JWTSecret))
	}
	if len(generated.SecretKeyBase) < 64 {
		t.Fatalf("secret key base length = %d, want at least 64", len(generated.SecretKeyBase))
	}
	if len(generated.VaultEncryptionKey) != 32 {
		t.Fatalf("vault encryption key length = %d, want exactly 32", len(generated.VaultEncryptionKey))
	}
}
