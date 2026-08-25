package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type ProjectSecrets struct {
	DatabasePassword   string
	JWTSecret          string
	AnonKey            string
	ServiceRoleKey     string
	DashboardPassword  string
	SecretKeyBase      string
	VaultEncryptionKey string
}

func Generate(random io.Reader) (ProjectSecrets, error) {
	databasePassword, err := randomString(random, 32)
	if err != nil {
		return ProjectSecrets{}, err
	}
	jwtSecret, err := randomString(random, 32)
	if err != nil {
		return ProjectSecrets{}, err
	}
	dashboardPassword, err := randomString(random, 24)
	if err != nil {
		return ProjectSecrets{}, err
	}
	secretKeyBase, err := randomString(random, 48)
	if err != nil {
		return ProjectSecrets{}, err
	}
	vaultKey, err := randomString(random, 32)
	if err != nil {
		return ProjectSecrets{}, err
	}
	now := time.Now().UTC()
	anon, err := signAPIKey(jwtSecret, "anon", now)
	if err != nil {
		return ProjectSecrets{}, err
	}
	serviceRole, err := signAPIKey(jwtSecret, "service_role", now)
	if err != nil {
		return ProjectSecrets{}, err
	}
	return ProjectSecrets{
		DatabasePassword:   databasePassword,
		JWTSecret:          jwtSecret,
		AnonKey:            anon,
		ServiceRoleKey:     serviceRole,
		DashboardPassword:  dashboardPassword,
		SecretKeyBase:      secretKeyBase,
		VaultEncryptionKey: vaultKey,
	}, nil
}

func VerifyAPIKey(token, secret, expectedRole string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("API key must have three JWT segments")
	}
	message := parts[0] + "." + parts[1]
	want := hmac.New(sha256.New, []byte(secret))
	_, _ = want.Write([]byte(message))
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, want.Sum(nil)) {
		return fmt.Errorf("API key signature is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode API key claims: %w", err)
	}
	var claims struct {
		Role string `json:"role"`
		Exp  int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("decode API key claims: %w", err)
	}
	if claims.Role != expectedRole {
		return fmt.Errorf("API key role is %q, want %q", claims.Role, expectedRole)
	}
	if claims.Exp <= time.Now().Unix() {
		return fmt.Errorf("API key is expired")
	}
	return nil
}

func signAPIKey(secret, role string, now time.Time) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, err := json.Marshal(map[string]any{
		"role": role,
		"iss":  "supabase-manager",
		"iat":  now.Unix(),
		"exp":  now.AddDate(10, 0, 0).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("encode API key claims: %w", err)
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	signature := hmac.New(sha256.New, []byte(secret))
	_, _ = signature.Write([]byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}

func randomString(random io.Reader, size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", fmt.Errorf("generate project secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
