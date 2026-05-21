package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"strings"
	"testing"
)

func TestSecureUniqueHelpers(t *testing.T) {
	a := SecureUniqueString()
	b := SecureUniqueString()
	if a == b {
		t.Fatal("expected unique random strings to differ")
	}
	if parts := strings.Split(SecureUniqueId("federation"), "@"); len(parts) != 2 || parts[1] != "federation" {
		t.Fatalf("unexpected unique id format: %v", parts)
	}
}

func TestSecureKeyPairsParseAndVerify(t *testing.T) {
	privPEM, pubPEM := SecureKeyPairs("")
	priv := ParsePrivateKey(privPEM)
	pub := ParsePublicKey(pubPEM)

	msg := []byte("caspar-signature-test")
	hash := sha256.Sum256(msg)
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, hash[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	if err != nil {
		t.Fatalf("failed signing: %v", err)
	}
	if err := rsa.VerifyPSS(pub, crypto.SHA256, hash[:], sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}); err != nil {
		t.Fatalf("failed verify: %v", err)
	}
}
