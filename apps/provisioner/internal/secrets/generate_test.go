package secrets

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestGenerateProducesDifferentSecretsPerProject(t *testing.T) {
	first, err := Generate(rand.Reader)
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	second, err := Generate(rand.Reader)
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if first.JWTSecret == second.JWTSecret || first.ServiceRoleKey == second.ServiceRoleKey || first.DatabasePassword == second.DatabasePassword {
		t.Fatal("Generate() reused a project secret")
	}
	if strings.Contains(first.AnonKey, "your-super-secret") || strings.Contains(first.ServiceRoleKey, "your-super-secret") {
		t.Fatal("Generate() returned an upstream placeholder")
	}
}

func TestGeneratedAPIKeysContainDistinctRolesAndValidSignatures(t *testing.T) {
	generated, err := Generate(rand.Reader)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := VerifyAPIKey(generated.AnonKey, generated.JWTSecret, "anon"); err != nil {
		t.Fatalf("anon key verification error = %v", err)
	}
	if err := VerifyAPIKey(generated.ServiceRoleKey, generated.JWTSecret, "service_role"); err != nil {
		t.Fatalf("service-role key verification error = %v", err)
	}
}

func TestGenerateProducesInternalRuntimeCredentials(t *testing.T) {
	generated, err := Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"realtime": generated.RealtimeDBEncryptionKey, "logflare-public": generated.LogflarePublicAccessToken, "logflare-private": generated.LogflarePrivateAccessToken, "s3-access": generated.S3ProtocolAccessKeyID, "s3-secret": generated.S3ProtocolAccessKeySecret, "pooler-tenant": generated.PoolerTenantID} {
		if value == "" {
			t.Errorf("%s secret is empty", name)
		}
	}
}
