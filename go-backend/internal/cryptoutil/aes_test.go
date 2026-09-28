package cryptoutil

import "testing"

func TestAESRoundTripAndRandomNonce(t *testing.T) {
	crypto, err := NewAES("node-secret")
	if err != nil {
		t.Fatalf("new AES: %v", err)
	}
	first, err := crypto.Encrypt([]byte(`{"type":"call"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := crypto.Encrypt([]byte(`{"type":"call"}`))
	if err != nil {
		t.Fatalf("encrypt second: %v", err)
	}
	if first == second {
		t.Fatal("AES-GCM nonce was reused")
	}
	plain, err := crypto.Decrypt(first)
	if err != nil || string(plain) != `{"type":"call"}` {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
}
