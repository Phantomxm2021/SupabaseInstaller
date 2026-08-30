package site

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var certificateNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CertificateInput is accepted only by the host-owned Nginx agent.
type CertificateInput struct {
	Name           string `json:"certificateName"`
	BaseDomain     string `json:"baseDomain"`
	CertificatePEM []byte `json:"certificatePem"`
	PrivateKeyPEM  []byte `json:"privateKeyPem"`
}

type CertificateResult struct {
	CertificateName string `json:"certificateName"`
	CertificateFile string `json:"certificateFile"`
	PrivateKeyFile  string `json:"privateKeyFile"`
	Created         bool   `json:"created"`
}

// CertificateStore is the only component allowed to write managed TLS files.
type CertificateStore struct{ directory string }

func NewCertificateStore(directory string) CertificateStore {
	return CertificateStore{directory: filepath.Clean(directory)}
}

func (s CertificateStore) Directory() string { return s.directory }

func (s CertificateStore) Stage(_ context.Context, input CertificateInput) (CertificateResult, error) {
	name, label, err := validateCertificateInput(input)
	if err != nil {
		return CertificateResult{}, err
	}
	certificate, err := parseCertificate(input.CertificatePEM)
	if err != nil {
		return CertificateResult{}, err
	}
	privateKey, err := parsePrivateKey(input.PrivateKeyPEM)
	if err != nil {
		return CertificateResult{}, err
	}
	if !samePublicKey(certificate.PublicKey, privateKey) {
		return CertificateResult{}, fmt.Errorf("certificate and private key do not match")
	}
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		return CertificateResult{}, fmt.Errorf("create certificate directory: %w", err)
	}
	if err := os.Chmod(s.directory, 0o755); err != nil {
		return CertificateResult{}, fmt.Errorf("set certificate directory permissions: %w", err)
	}
	base := name + "-" + label
	result := CertificateResult{
		CertificateName: name,
		CertificateFile: filepath.Join(s.directory, base+".pem"),
		PrivateKeyFile:  filepath.Join(s.directory, base+".key"),
	}
	existingCertificate, certificateExists, err := readOptionalFile(result.CertificateFile)
	if err != nil {
		return CertificateResult{}, err
	}
	existingKey, keyExists, err := readOptionalFile(result.PrivateKeyFile)
	if err != nil {
		return CertificateResult{}, err
	}
	if certificateExists || keyExists {
		if !certificateExists || !keyExists || !bytes.Equal(existingCertificate, input.CertificatePEM) || !bytes.Equal(existingKey, input.PrivateKeyPEM) {
			return CertificateResult{}, fmt.Errorf("managed certificate already exists with different material")
		}
		return result, nil
	}
	if err := writeAtomic(s.directory, base+".pem", input.CertificatePEM, 0o644); err != nil {
		return CertificateResult{}, fmt.Errorf("publish certificate: %w", err)
	}
	if err := writeAtomic(s.directory, base+".key", input.PrivateKeyPEM, 0o600); err != nil {
		_ = removeIfPresent(result.CertificateFile)
		return CertificateResult{}, fmt.Errorf("publish private key: %w", err)
	}
	result.Created = true
	return result, nil
}

func validateCertificateInput(input CertificateInput) (string, string, error) {
	name := strings.TrimSpace(strings.ToLower(input.Name))
	if !certificateNamePattern.MatchString(name) {
		return "", "", fmt.Errorf("invalid certificate name")
	}
	baseDomain := strings.TrimSpace(strings.ToLower(input.BaseDomain))
	labels := strings.Split(baseDomain, ".")
	if len(labels) < 2 {
		return "", "", fmt.Errorf("invalid base domain")
	}
	for _, domainLabel := range labels {
		if !certificateNamePattern.MatchString(domainLabel) {
			return "", "", fmt.Errorf("invalid base domain")
		}
	}
	if len(input.CertificatePEM) == 0 || len(input.PrivateKeyPEM) == 0 {
		return "", "", fmt.Errorf("certificate and private key are required")
	}
	return name, labels[0], nil
}

func parseCertificate(contents []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate PEM: %w", err)
	}
	// Nginx accepts a leaf certificate followed by intermediate certificates.
	// Validate every block but keep the first leaf to compare its public key.
	for len(bytes.TrimSpace(rest)) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("invalid certificate PEM chain")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, fmt.Errorf("parse certificate PEM chain: %w", err)
		}
	}
	return certificate, nil
}

func parsePrivateKey(contents []byte) (crypto.PublicKey, error) {
	block, rest := pem.Decode(contents)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("invalid private key PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return publicKey(key)
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key.Public(), nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key.Public(), nil
	}
	return nil, fmt.Errorf("parse private key PEM: unsupported key format")
}

func publicKey(key any) (crypto.PublicKey, error) {
	switch key := key.(type) {
	case *rsa.PrivateKey:
		return key.Public(), nil
	case *ecdsa.PrivateKey:
		return key.Public(), nil
	case ed25519.PrivateKey:
		return key.Public(), nil
	default:
		return nil, fmt.Errorf("parse private key PEM: unsupported key type")
	}
}

func samePublicKey(certificateKey, privateKey crypto.PublicKey) bool {
	certificateDER, certificateErr := x509.MarshalPKIXPublicKey(certificateKey)
	privateDER, privateErr := x509.MarshalPKIXPublicKey(privateKey)
	return certificateErr == nil && privateErr == nil && bytes.Equal(certificateDER, privateDER)
}

func readOptionalFile(path string) ([]byte, bool, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read managed certificate: %w", err)
	}
	return contents, true, nil
}
