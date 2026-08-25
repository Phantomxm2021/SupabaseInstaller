package install

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"supabase-manager/internal/contracts"
)

type CryptoGenerator struct {
	Random io.Reader
	Now    func() time.Time
}

func (generator CryptoGenerator) Generate() (contracts.ProjectSecrets, error) {
	random := generator.Random
	if random == nil {
		random = rand.Reader
	}
	now := generator.Now
	if now == nil {
		now = time.Now
	}
	database, err := generatedValue(random, 32)
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	jwtSecret, err := generatedValue(random, 32)
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	dashboard, err := generatedValue(random, 24)
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	secretKeyBase, err := generatedValue(random, 48)
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	vaultKey, err := generatedValue(random, 32)
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	anon, err := generatedAPIKey(jwtSecret, "anon", now())
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	serviceRole, err := generatedAPIKey(jwtSecret, "service_role", now())
	if err != nil {
		return contracts.ProjectSecrets{}, err
	}
	return contracts.ProjectSecrets{
		DatabasePassword: database, JWTSecret: jwtSecret, AnonKey: anon, ServiceRoleKey: serviceRole,
		DashboardPassword: dashboard, SecretKeyBase: secretKeyBase, VaultEncryptionKey: vaultKey,
	}, nil
}

func generatedValue(random io.Reader, size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", fmt.Errorf("generate runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func generatedAPIKey(secret, role string, now time.Time) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, err := json.Marshal(map[string]any{"role": role, "iss": "supabase-manager", "iat": now.Unix(), "exp": now.AddDate(10, 0, 0).Unix()})
	if err != nil {
		return "", err
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	signature := hmac.New(sha256.New, []byte(secret))
	_, _ = signature.Write([]byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}
