package auth

import "testing"

func TestPasswordConfigured(t *testing.T) {
	hash, err := HashPassword("a-configured-password")
	if err != nil {
		t.Fatal(err)
	}
	if !PasswordConfigured(hash) {
		t.Fatal("valid password not recognized")
	}
	for _, value := range []string{"", "disabled-until-reset", "not-a-password-hash"} {
		if PasswordConfigured(value) {
			t.Fatalf("placeholder recognized as password: %q", value)
		}
	}
}
