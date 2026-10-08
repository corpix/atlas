package crypto

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"hash"
	"testing"

	"git.tatikoma.dev/corpix/atlas/errors"
)

func TestTokenRoundTrip(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		newHash        func() hash.Hash
		signatureBytes int
	}{
		{name: "sha256", newHash: sha256.New, signatureBytes: sha256.Size},
		{name: "sha512", newHash: sha512.New, signatureBytes: sha512.Size},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				token   = NewToken(test.newHash)
				key     = []byte("key")
				payload = []byte("payload")
				meta    = [][]byte{[]byte("first"), nil, []byte("third")}
			)
			value := token.Sign(key, payload, meta...)
			parsed, signature, err := token.Parse(value)
			if err != nil {
				t.Fatal(err)
			}
			if string(parsed) != string(payload) {
				t.Fatalf("payload = %q, want %q", parsed, payload)
			}
			if len(signature) != test.signatureBytes {
				t.Fatalf("signature length = %d, want %d", len(signature), test.signatureBytes)
			}
			if !token.Verify(key, parsed, signature, meta...) {
				t.Fatal("signature did not verify")
			}
		})
	}
}

func TestTokenVerifyRejectsChanges(t *testing.T) {
	t.Parallel()

	var (
		token   = NewToken(sha256.New)
		key     = []byte("key")
		payload = []byte("payload")
		meta    = [][]byte{[]byte("ab"), []byte("c")}
	)
	value := token.Sign(key, payload, meta...)
	parsed, signature, err := token.Parse(value)
	if err != nil {
		t.Fatal(err)
	}

	changedSignature := append([]byte(nil), signature...)
	changedSignature[0] ^= 0xff
	for name, valid := range map[string]bool{
		"key":               token.Verify([]byte("other"), parsed, signature, meta...),
		"payload":           token.Verify(key, []byte("other"), signature, meta...),
		"signature":         token.Verify(key, parsed, changedSignature, meta...),
		"metadata":          token.Verify(key, parsed, signature, []byte("other")),
		"metadata order":    token.Verify(key, parsed, signature, meta[1], meta[0]),
		"metadata boundary": token.Verify(key, parsed, signature, []byte("a"), []byte("bc")),
	} {
		if valid {
			t.Errorf("%s change verified", name)
		}
	}
}

func TestTokenParseRejectsMalformedValues(t *testing.T) {
	t.Parallel()

	token := NewToken(sha256.New)
	for _, value := range []string{
		"",
		"not-base64",
		base64.RawURLEncoding.EncodeToString([]byte{tokenVersion}),
		base64.RawURLEncoding.EncodeToString(make([]byte, 1+sha256.Size)),
	} {
		_, _, err := token.Parse(value)
		if !errors.Is(err, ErrTokenInvalid) {
			t.Errorf("Parse(%q) error = %v, want %v", value, err, ErrTokenInvalid)
		}
	}
}
