package security

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("test-master-key-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	secret := "sk-test-1234567890"
	enc, err := c.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	if enc == secret || enc == "" {
		t.Fatal("secret was not encrypted")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != secret {
		t.Fatalf("round trip mismatch: %q", dec)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("abcdef1234567890"); got != "abcd••••••••7890" {
		t.Fatalf("unexpected mask: %s", got)
	}
	if got := MaskSecret(""); got != "" {
		t.Fatalf("empty secret should be empty, got %q", got)
	}
}
