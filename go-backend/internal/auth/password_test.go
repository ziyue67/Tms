package auth

import "testing"

func TestLegacyMD5PasswordCanBeUpgraded(t *testing.T) {
	valid, upgrade := VerifyPassword("3c85cdebade1c51cf64ca9f3c09d182d", "admin_user")
	if !valid || !upgrade {
		t.Fatalf("legacy password result: valid=%v upgrade=%v", valid, upgrade)
	}
}

func TestBcryptPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	valid, upgrade := VerifyPassword(hash, "correct horse battery staple")
	if !valid || upgrade {
		t.Fatalf("bcrypt result: valid=%v upgrade=%v", valid, upgrade)
	}
}
