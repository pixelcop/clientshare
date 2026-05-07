package auth

import (
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var AuthExpiry = 30 * 24 * time.Hour

var jwtSecret []byte

func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}

// ParseJWT parses and validates a JWT token
func ParseJWT(tokenStr string) (*jwt.Token, error) {
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	return jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
}

// GenerateJWT generates a JWT token for an authenticated user.
func GenerateJWT(userID string, role string, tenantID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":   userID,
		"role":      role,
		"tenant_id": tenantID,
		"exp":       time.Now().Add(AuthExpiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// GenerateLinkJWT generates a JWT for secure link access
func GenerateLinkJWT(clientID string, linkID string, accessType string, tenantID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":     "",
		"role":        "link",
		"tenant_id":   tenantID,
		"client_id":   clientID,
		"link_id":     linkID,
		"access_type": accessType,
		"exp":         time.Now().Add(AuthExpiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}
