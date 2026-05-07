package links

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"github.com/google/uuid"
	"strings"
	"time"
)

// GenerateSecureToken creates a UUID + HMAC signature token
func GenerateSecureToken(signingKey string) (string, error) {
	id := uuid.New().String()
	h := hmac.New(sha256.New, []byte(signingKey))
	h.Write([]byte(id))
	sig := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return id + "." + sig, nil
}

// ValidateSecureToken checks HMAC and returns UUID if valid
func ValidateSecureToken(token, signingKey string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	id, sig := parts[0], parts[1]
	h := hmac.New(sha256.New, []byte(signingKey))
	h.Write([]byte(id))
	expected := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return id, hmac.Equal([]byte(sig), []byte(expected))
}

// IsExpired checks if expiry time is in the past
func IsExpired(expiry time.Time) bool {
	return time.Now().After(expiry)
}
