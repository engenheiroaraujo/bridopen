package key

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

func GenerateTokenKey() (string, error) {
	r := make([]byte, 32)
	if _, err := rand.Read(r); err != nil {
		return "", fmt.Errorf("falha ao gerar token seguro: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(r), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
