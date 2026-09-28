package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("无效的token或token已过期")
	errMalformedJWT = errors.New("malformed JWT")
)

type Claims struct {
	Subject string `json:"sub"`
	Issued  int64  `json:"iat"`
	Expiry  int64  `json:"exp"`
	User    string `json:"user"`
	Name    string `json:"name"`
	RoleID  int    `json:"role_id"`
}

func (c Claims) UserID() (int64, error) {
	return strconv.ParseInt(c.Subject, 10, 64)
}

type TokenService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (s *TokenService) Generate(userID int64, username string, roleID int) (string, error) {
	now := s.now()
	claims := Claims{
		Subject: strconv.FormatInt(userID, 10),
		Issued:  now.Unix(),
		Expiry:  now.Add(s.ttl).Unix(),
		User:    username,
		Name:    username,
		RoleID:  roleID,
	}
	header := struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}{Algorithm: "HmacSHA256", Type: "JWT"}

	encodedHeader, err := encodeJSON(header)
	if err != nil {
		return "", fmt.Errorf("encode JWT header: %w", err)
	}
	encodedClaims, err := encodeJSON(claims)
	if err != nil {
		return "", fmt.Errorf("encode JWT claims: %w", err)
	}
	content := encodedHeader + "." + encodedClaims
	return content + "." + s.signature(content), nil
}

func (s *TokenService) Validate(raw string) (Claims, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		raw = strings.TrimSpace(raw[7:])
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	expected := s.signature(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || (header.Algorithm != "HmacSHA256" && header.Algorithm != "HS256") {
		return Claims{}, ErrInvalidToken
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil || claims.Subject == "" || claims.Expiry <= s.now().Unix() {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (s *TokenService) signature(content string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(content))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeJSON(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", errMalformedJWT
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
