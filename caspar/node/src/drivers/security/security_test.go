package security

import (
	"testing"

	cryp "kasper/src/shell/utils/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	privateKey, publicKey := cryp.SecureKeyPairs("")
	sm := &Security{
		keys: map[string][][]byte{
			"unit": {privateKey, publicKey},
		},
	}

	plain := "hello-caspar"
	cipher := sm.Encrypt("unit", plain)
	if cipher == "" {
		t.Fatal("expected non-empty ciphertext")
	}

	got := sm.Decrypt("unit", cipher)
	if got != plain {
		t.Fatalf("decrypt mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptRejectsInvalidHexCiphertext(t *testing.T) {
	privateKey, publicKey := cryp.SecureKeyPairs("")
	sm := &Security{
		keys: map[string][][]byte{
			"unit": {privateKey, publicKey},
		},
	}

	got := sm.Decrypt("unit", "not-hex")
	if got != "" {
		t.Fatalf("expected empty result for invalid cipher text, got %q", got)
	}
}
