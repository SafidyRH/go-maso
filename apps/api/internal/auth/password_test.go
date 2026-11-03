package auth

import "testing"

func TestPasswordHashAndVerification(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("VerifyPassword() rejected the correct password")
	}
	if VerifyPassword(hash, "incorrect password") {
		t.Fatal("VerifyPassword() accepted an incorrect password")
	}
}

func TestPasswordValidation(t *testing.T) {
	t.Parallel()

	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("HashPassword() accepted a short password")
	}
	if VerifyPassword("malformed", "password") {
		t.Fatal("VerifyPassword() accepted a malformed hash")
	}
}
