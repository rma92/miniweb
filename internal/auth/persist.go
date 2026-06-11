package auth

import (
	"os"
	"strings"
)

// LoadOrGenerateToken loads the persisted token from path, or generates a new
// one, writes it to path, and returns it. Returns the token and whether it was
// freshly generated.
func LoadOrGenerateToken(path string) (token string, fresh bool, err error) {
	data, err := os.ReadFile(path)
	if err == nil {
		tok := strings.TrimSpace(string(data))
		if tok != "" {
			return tok, false, nil
		}
	}

	// Generate a new token and persist it.
	tok, err := GenerateToken()
	if err != nil {
		return "", false, err
	}
	if writeErr := os.WriteFile(path, []byte(tok+"\n"), 0600); writeErr != nil {
		// Non-fatal: token works this session but won't survive restart.
		_ = writeErr
	}
	return tok, true, nil
}
