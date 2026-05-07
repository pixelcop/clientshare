//go:build !testmain
// +build !testmain

package links

import (
	"testing"
	"time"
)

func TestGenerateAndValidateSecureToken(t *testing.T) {
	key := "testkey"
	token, err := GenerateSecureToken(key)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	id, valid := ValidateSecureToken(token, key)
	if !valid || id == "" {
		t.Errorf("token should be valid, got valid=%v id=%s", valid, id)
	}
	_, valid2 := ValidateSecureToken(token, "wrongkey")
	if valid2 {
		t.Errorf("token should be invalid with wrong key")
	}
}

func TestIsExpired(t *testing.T) {
	if !IsExpired(time.Now().Add(-time.Hour)) {
		t.Error("should be expired")
	}
	if IsExpired(time.Now().Add(time.Hour)) {
		t.Error("should not be expired")
	}
}
