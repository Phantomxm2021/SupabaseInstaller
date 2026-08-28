package site

import (
	"crypto/md5" // #nosec G501 -- Apache apr1 is the Nginx-compatible htpasswd format used by the official template.
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	defaultAuthDirectory = "/etc/supabase-manager/nginx-auth"
	cryptAlphabet        = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

func renderHTPasswd(username, password string) (string, error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return "", fmt.Errorf("Studio credentials are required")
	}
	if strings.ContainsAny(username, ":\r\n") {
		return "", fmt.Errorf("invalid Studio username")
	}
	salt, err := randomSalt(8)
	if err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	return username + ":" + apr1Crypt(password, salt) + "\n", nil
}

func randomSalt(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("salt length must be positive")
	}
	bytes := make([]byte, length)
	for index := range bytes {
		for {
			var random [1]byte
			if _, err := rand.Read(random[:]); err != nil {
				return "", err
			}
			limit := 256 - (256 % len(cryptAlphabet))
			if int(random[0]) < limit {
				bytes[index] = cryptAlphabet[int(random[0])%len(cryptAlphabet)]
				break
			}
		}
	}
	return string(bytes), nil
}

// apr1Crypt implements Apache's MD5 password format. Nginx's auth_basic
// supports it directly, and the upstream Supabase Nginx template creates the
// same format with `openssl passwd -apr1`.
func apr1Crypt(password, salt string) string {
	passwordBytes := []byte(password)
	salt = strings.TrimPrefix(salt, "$apr1$")
	if delimiter := strings.IndexByte(salt, '$'); delimiter >= 0 {
		salt = salt[:delimiter]
	}
	if len(salt) > 8 {
		salt = salt[:8]
	}

	initial := md5.New() // #nosec G401 -- required by the interoperable apr1 format.
	_, _ = initial.Write(passwordBytes)
	_, _ = initial.Write([]byte("$apr1$"))
	_, _ = initial.Write([]byte(salt))

	alternate := md5.Sum(append(append(append([]byte{}, passwordBytes...), []byte(salt)...), passwordBytes...)) // #nosec G401 -- required by apr1.
	for remaining := len(passwordBytes); remaining > 0; remaining -= len(alternate) {
		count := remaining
		if count > len(alternate) {
			count = len(alternate)
		}
		_, _ = initial.Write(alternate[:count])
	}
	for length := len(passwordBytes); length > 0; length >>= 1 {
		if length&1 != 0 {
			_, _ = initial.Write([]byte{0})
		} else if len(passwordBytes) > 0 {
			_, _ = initial.Write(passwordBytes[:1])
		}
	}
	digest := initial.Sum(nil)

	for round := 0; round < 1000; round++ {
		hash := md5.New() // #nosec G401 -- required by apr1.
		if round&1 != 0 {
			_, _ = hash.Write(passwordBytes)
		} else {
			_, _ = hash.Write(digest)
		}
		if round%3 != 0 {
			_, _ = hash.Write([]byte(salt))
		}
		if round%7 != 0 {
			_, _ = hash.Write(passwordBytes)
		}
		if round&1 != 0 {
			_, _ = hash.Write(digest)
		} else {
			_, _ = hash.Write(passwordBytes)
		}
		digest = hash.Sum(nil)
	}

	encoded := make([]byte, 0, 22)
	encoded = appendCrypt64(encoded, uint32(digest[0])<<16|uint32(digest[6])<<8|uint32(digest[12]), 4)
	encoded = appendCrypt64(encoded, uint32(digest[1])<<16|uint32(digest[7])<<8|uint32(digest[13]), 4)
	encoded = appendCrypt64(encoded, uint32(digest[2])<<16|uint32(digest[8])<<8|uint32(digest[14]), 4)
	encoded = appendCrypt64(encoded, uint32(digest[3])<<16|uint32(digest[9])<<8|uint32(digest[15]), 4)
	encoded = appendCrypt64(encoded, uint32(digest[4])<<16|uint32(digest[10])<<8|uint32(digest[5]), 4)
	encoded = appendCrypt64(encoded, uint32(digest[11]), 2)
	return "$apr1$" + salt + "$" + string(encoded)
}

func appendCrypt64(destination []byte, value uint32, count int) []byte {
	for ; count > 0; count-- {
		destination = append(destination, cryptAlphabet[value&0x3f])
		value >>= 6
	}
	return destination
}
