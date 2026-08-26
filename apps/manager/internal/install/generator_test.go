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
	for name, value := range map[string]string{"realtime": generated.RealtimeDBEncryptionKey, "logflare-public": generated.LogflarePublicAccessToken, "logflare-private": generated.LogflarePrivateAccessToken, "s3-access": generated.S3ProtocolAccessKeyID, "s3-secret": generated.S3ProtocolAccessKeySecret, "pooler-tenant": generated.PoolerTenantID} {
		if value == "" {
			t.Errorf("%s secret is empty", name)
		}
	}
	if len(generated.RealtimeDBEncryptionKey) != 16 {
		t.Fatalf("realtime encryption key length = %d, want 16", len(generated.RealtimeDBEncryptionKey))
	}
}
