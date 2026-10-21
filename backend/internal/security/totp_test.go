package security

import (
	"testing"
	"time"
)

func TestTOTPRFC6238Vector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, 20-byte secret "12345678901234567890".
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(59, 0)
	if code := totpCode(secret, at.Unix()/30); code != "287082" {
		t.Fatalf("totpCode = %s, want 287082", code)
	}
	if !ValidateTOTP(secret, "287082", at) {
		t.Fatal("expected RFC vector to validate")
	}
	if !ValidateTOTP(secret, "287082", at.Add(29*time.Second)) {
		t.Fatal("expected next step to validate the same code within skew")
	}
	if ValidateTOTP(secret, "000000", at) {
		t.Fatal("wrong code must fail")
	}
}

func TestGenerateTOTPSecret(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil || len(secret) != 32 {
		t.Fatalf("unexpected secret %q err %v", secret, err)
	}
	if url := TOTPProvisioningURL(secret, "alice", "SmartDoc Vision"); url == "" {
		t.Fatal("empty provisioning URL")
	}
}
