package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

type AES struct {
	aead cipher.AEAD
}

func NewAES(secret string) (*AES, error) {
	if secret == "" {
		return nil, errors.New("密钥不能为空")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AES{aead: aead}, nil
}

func (a *AES) Encrypt(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", errors.New("待加密数据不能为空")
	}
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := a.aead.Seal(nil, nonce, plaintext, nil)
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

func (a *AES) Decrypt(encoded string) ([]byte, error) {
	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	if len(combined) < a.aead.NonceSize() {
		return nil, errors.New("加密数据长度不足")
	}
	nonce := combined[:a.aead.NonceSize()]
	return a.aead.Open(nil, nonce, combined[a.aead.NonceSize():], nil)
}
