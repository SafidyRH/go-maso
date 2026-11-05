package incident

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateSigningSecretHasEnoughEntropy(t *testing.T) {
	first, err := generateSigningSecret()
	if err != nil {
		t.Fatalf("generateSigningSecret() error = %v", err)
	}
	second, err := generateSigningSecret()
	if err != nil {
		t.Fatalf("generateSigningSecret() second error = %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode signing secret: %v", err)
	}
	if len(decoded) != signingSecretBytes {
		t.Fatalf("decoded secret length = %d, want %d", len(decoded), signingSecretBytes)
	}
	if first == second {
		t.Fatal("generated signing secrets are identical")
	}
}

func TestWebhookSignatureIsVersioned(t *testing.T) {
	signature := webhookSignature("secret", "1700000000", []byte(`{"ok":true}`))
	if !strings.HasPrefix(signature, "v1=") || len(signature) != 67 {
		t.Fatalf("webhookSignature() = %q", signature)
	}
}
