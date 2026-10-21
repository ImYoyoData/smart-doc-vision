package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func SignHMAC(secret, canonical string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func BodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func SignatureEqual(expected, got string) bool {
	return hmac.Equal([]byte(expected), []byte(got))
}
