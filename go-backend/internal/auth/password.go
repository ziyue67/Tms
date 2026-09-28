package auth

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func VerifyPassword(encoded, plain string) (valid bool, needsUpgrade bool) {
	if strings.HasPrefix(encoded, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(plain)) == nil, false
	}
	digest := md5.Sum([]byte(plain)) // Compatibility with passwords stored by the legacy backend.
	want := hex.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(encoded)), []byte(want)) == 1, true
}

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}
