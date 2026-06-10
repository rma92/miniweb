package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateToken returns a cryptographically random 32-byte base64url token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
