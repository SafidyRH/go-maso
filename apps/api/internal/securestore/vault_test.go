package securestore

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(fill byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(fill), 32)))
}

func TestVaultRoundTripAndPurposeBinding(t *testing.T) {
	t.Parallel()

	vault, err := NewVault(testKey('a'))
	if err != nil {
		t.Fatalf("NewVault() error = %v", err)
	}
	encrypted, err := vault.EncryptString("https://hooks.example.com/token", "webhook")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if strings.Contains(string(encrypted), "hooks.example.com") {
		t.Fatal("encrypted value contains plaintext")
	}
	decrypted, err := vault.DecryptString(encrypted, "webhook")
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decrypted != "https://hooks.example.com/token" {
		t.Fatalf("decrypted value = %q", decrypted)
	}
	if _, err := vault.DecryptString(encrypted, "another-purpose"); err == nil {
		t.Fatal("DecryptString() accepted a different purpose")
	}
}

func TestVaultRejectsInvalidKeysAndTampering(t *testing.T) {
	t.Parallel()

	if _, err := NewVault(""); err == nil {
		t.Fatal("NewVault() accepted an empty key")
	}
	if _, err := NewVault(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("NewVault() accepted a short key")
	}

	vault, _ := NewVault(testKey('b'))
	encrypted, _ := vault.EncryptString("secret", "purpose")
	encrypted[len(encrypted)-1] ^= 1
	if _, err := vault.DecryptString(encrypted, "purpose"); err == nil {
		t.Fatal("DecryptString() accepted tampered ciphertext")
	}
}
