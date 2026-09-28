package payment

import (
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/url"
	"sort"
	"strings"
)

func orderedQuery(values map[string]string, encoded bool) string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if value != "" && !strings.EqualFold(key, "sign") && !strings.EqualFold(key, "sign_type") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		if encoded {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		} else {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, "&")
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hmacHex(key, value string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func constantEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func signRSA(value, pemValue string) (string, error) {
	key, err := privateKey(pemValue)
	if err != nil {
		return "", errors.New("支付私钥配置无效")
	}
	digest := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyRSA(value, signature, pemValue string) bool {
	key, err := publicKey(pemValue)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(value))
	decoded, err := base64.StdEncoding.DecodeString(signature)
	return err == nil && rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], decoded) == nil
}

func privateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
		if err != nil {
			return nil, err
		}
		block = &pem.Block{Bytes: decoded}
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func publicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
		if err != nil {
			return nil, err
		}
		block = &pem.Block{Bytes: decoded}
	}
	if certificate, err := x509.ParseCertificate(block.Bytes); err == nil {
		key, ok := certificate.PublicKey.(*rsa.PublicKey)
		if ok {
			return key, nil
		}
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return key, nil
}
