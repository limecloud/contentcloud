package idgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/google/uuid"
)

func New() string {
	value, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return value.String()
}

// Deterministic returns a UUID-shaped identifier for a stable business key.
// It is used where retries must converge on one durable fact without adding
// a second idempotency table.
func Deterministic(key string) string {
	sum := sha256.Sum256([]byte(key))
	value := uuid.UUID(sum[:16])
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	return value.String()
}

func NewOpaqueToken(prefix string, size int) (plain string, hash string, err error) {
	value := make([]byte, size)
	if _, err = rand.Read(value); err != nil {
		return "", "", err
	}
	plain = prefix + base64.RawURLEncoding.EncodeToString(value)
	return plain, TokenHash(plain), nil
}

func TokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
