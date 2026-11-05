package incident

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
)

const signingSecretBytes = 32

func generateSigningSecret() (string, error) {
	value := make([]byte, signingSecretBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate webhook signing secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func webhookSignature(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookTimestamp(unixSeconds int64) string {
	return strconv.FormatInt(unixSeconds, 10)
}
