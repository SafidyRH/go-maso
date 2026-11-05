package securestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const envelopeVersion byte = 1

type Vault struct {
	aead cipher.AEAD
}

func NewVault(encodedKey string) (*Vault, error) {
	if encodedKey == "" {
		return nil, errors.New("SECRETS_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("SECRETS_ENCRYPTION_KEY must be standard base64")
	}
	if len(key) != 32 {
		return nil, errors.New("SECRETS_ENCRYPTION_KEY must decode to exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) EncryptString(plaintext, purpose string) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(plaintext)+v.aead.Overhead())
	envelope[0] = envelopeVersion
	envelope = append(envelope, nonce...)
	envelope = v.aead.Seal(envelope, nonce, []byte(plaintext), []byte(purpose))
	return envelope, nil
}

func (v *Vault) DecryptString(envelope []byte, purpose string) (string, error) {
	if len(envelope) < 1+v.aead.NonceSize()+v.aead.Overhead() || envelope[0] != envelopeVersion {
		return "", errors.New("encrypted secret envelope is invalid")
	}
	nonceEnd := 1 + v.aead.NonceSize()
	nonce := envelope[1:nonceEnd]
	plaintext, err := v.aead.Open(nil, nonce, envelope[nonceEnd:], []byte(purpose))
	if err != nil {
		return "", errors.New("decrypt secret: authentication failed")
	}
	return string(plaintext), nil
}
