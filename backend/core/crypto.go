package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

const encPrefix = "enc:v1:"

// Cipher encrypts config_json with AES-256-GCM.
type Cipher struct{ gcm cipher.AEAD }

// NewCipher takes a base64-encoded 32-byte key (e.g. `openssl rand -base64 32`).
func NewCipher(b64Key string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil || len(key) != 32 {
		return nil, errors.New("encryption key must be base64 of exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	return &Cipher{gcm}, err
}

func IsEncrypted(s string) bool { return strings.HasPrefix(s, encPrefix) }

func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return encPrefix + base64.StdEncoding.EncodeToString(c.gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (c *Cipher) Decrypt(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, encPrefix))
	if err != nil || len(raw) < c.gcm.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	n := c.gcm.NonceSize()
	plain, err := c.gcm.Open(nil, raw[:n], raw[n:], nil)
	return string(plain), err
}
