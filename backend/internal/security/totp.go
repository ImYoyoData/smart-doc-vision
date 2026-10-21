package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a fresh 160-bit base32 secret suitable for
// authenticator apps (Google Authenticator, Microsoft Authenticator, 1Password).
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32NoPadding.EncodeToString(raw), nil
}

// TOTPProvisioningURL returns an otpauth:// URI that authenticator apps can
// scan or import.
func TOTPProvisioningURL(secret, account, issuer string) string {
	if issuer == "" {
		issuer = "SmartDoc Vision"
	}
	if account == "" {
		account = "admin"
	}
	label := issuer + ":" + account
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		escapeTOTPLabel(label), secret, escapeTOTPLabel(issuer))
}

func escapeTOTPLabel(value string) string {
	return strings.NewReplacer(" ", "%20", ":", "%3A", "/", "%2F", "?", "%3F", "&", "%26").Replace(value)
}

// ValidateTOTP checks a six-digit code across the current 30-second window
// with one step of clock skew in either direction.
func ValidateTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	for offset := int64(-1); offset <= 1; offset++ {
		if totpCode(secret, now.Unix()/30+offset) == code {
			return true
		}
	}
	return false
}

func totpCode(secret string, counter int64) string {
	key, err := base32NoPadding.DecodeString(secret)
	if err != nil {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", binaryCode%1000000)
}
