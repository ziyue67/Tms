package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLegacyCompatibleJWT(t *testing.T) {
	service := NewTokenService("test-secret", 90*24*time.Hour)
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	token, err := service.Generate(42, "alice", 1)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "HmacSHA256" {
		t.Fatalf("legacy Java algorithm label changed: %q", header["alg"])
	}

	claims, err := service.Validate(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Subject != "42" || claims.Name != "alice" || claims.RoleID != 1 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestRejectsExpiredOrModifiedJWT(t *testing.T) {
	service := NewTokenService("test-secret", time.Second)
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	token, err := service.Generate(1, "admin", 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	service.now = func() time.Time { return time.Unix(1_700_000_002, 0) }
	if _, err := service.Validate(token); err == nil {
		t.Fatal("expired token was accepted")
	}
	if _, err := service.Validate(token + "x"); err == nil {
		t.Fatal("modified token was accepted")
	}
}
