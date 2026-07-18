package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		plaintext []byte
		wantErr   bool
	}{
		{
			name:      "valid key and text",
			key:       bytes.Repeat([]byte{0x01}, 32),
			plaintext: []byte("my super secret kubeconfig"),
		},
		{
			name:      "empty plaintext",
			key:       bytes.Repeat([]byte{0x02}, 32),
			plaintext: []byte{},
		},
		{
			name:      "large plaintext",
			key:       bytes.Repeat([]byte{0x03}, 32),
			plaintext: bytes.Repeat([]byte("A"), 10000),
		},
		{
			name:      "invalid key length",
			key:       bytes.Repeat([]byte{0x01}, 16),
			plaintext: []byte("test"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := Encrypt(tt.key, tt.plaintext)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}

			// ciphertext must differ from plaintext
			if bytes.Equal(ct, tt.plaintext) && len(tt.plaintext) > 0 {
				t.Fatal("ciphertext equals plaintext")
			}

			pt, err := Decrypt(tt.key, ct)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(pt, tt.plaintext) {
				t.Fatalf("round-trip mismatch: got %q, want %q", pt, tt.plaintext)
			}
		})
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := bytes.Repeat([]byte{0x01}, 32)
	key2 := bytes.Repeat([]byte{0x02}, 32)

	ct, err := Encrypt(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if _, err := Decrypt(key2, ct); err == nil {
		t.Fatal("expected decryption error with wrong key, got nil")
	}
}

func TestEncrypt_NonUniqueness(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	pt := []byte("same input")

	ct1, _ := Encrypt(key, pt)
	ct2, _ := Encrypt(key, pt)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of same plaintext produced identical ciphertext (nonce reuse?)")
	}
}

func TestParseDEK(t *testing.T) {
	// Generate a valid 32-byte key
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	hexKey := hex.EncodeToString(raw)

	key, err := ParseDEK(hexKey)
	if err != nil {
		t.Fatalf("ParseDEK: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length: got %d, want 32", len(key))
	}

	// Invalid hex
	if _, err := ParseDEK("not-hex"); err == nil {
		t.Fatal("expected error for invalid hex")
	}

	// Wrong length (16 bytes = 32 hex chars)
	shortKey := hex.EncodeToString(raw[:16])
	if _, err := ParseDEK(shortKey); err == nil {
		t.Fatal("expected error for short key")
	}
}
