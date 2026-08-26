package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const envelopeVersion = 1

type Envelope struct {
	Version    int
	Nonce      []byte
	Ciphertext []byte
}

type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM requires a 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(projectID, kind string, plaintext []byte) (Envelope, error) {
	if projectID == "" || kind == "" {
		return Envelope{}, fmt.Errorf("project ID and secret kind are required")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Envelope{}, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plaintext, additionalData(projectID, kind))
	return Envelope{Version: envelopeVersion, Nonce: nonce, Ciphertext: ciphertext}, nil
}

func (c *Cipher) Decrypt(projectID, kind string, envelope Envelope) ([]byte, error) {
	if envelope.Version != envelopeVersion {
		return nil, fmt.Errorf("unsupported secret envelope version %d", envelope.Version)
	}
	plaintext, err := c.aead.Open(nil, envelope.Nonce, envelope.Ciphertext, additionalData(projectID, kind))
	if err != nil {
		return nil, fmt.Errorf("authenticate secret: %w", err)
	}
	return plaintext, nil
}

func additionalData(projectID, kind string) []byte {
	return []byte(fmt.Sprintf("v%d:%s:%s", envelopeVersion, projectID, kind))
}
