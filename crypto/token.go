package crypto

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/binary"
	"hash"

	"git.tatikoma.dev/corpix/atlas/errors"
)

const tokenVersion = byte(1)

var ErrTokenInvalid = errors.New("token is invalid")

type Token struct {
	newHash        func() hash.Hash
	signatureBytes int
}

func (t *Token) Sign(key, payload []byte, meta ...[]byte) string {
	out := make([]byte, 1, 1+len(payload)+t.signatureBytes)
	out[0] = tokenVersion
	out = append(out, payload...)
	out = append(out, t.signature(key, payload, meta...)...)
	return base64.RawURLEncoding.EncodeToString(out)
}

func (t *Token) Parse(value string) ([]byte, []byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) < 1+t.signatureBytes || data[0] != tokenVersion {
		return nil, nil, ErrTokenInvalid
	}

	signatureOffset := len(data) - t.signatureBytes
	return data[1:signatureOffset], data[signatureOffset:], nil
}

func (t *Token) Verify(key, payload, signature []byte, meta ...[]byte) bool {
	return hmac.Equal(signature, t.signature(key, payload, meta...))
}

func (t *Token) signature(key, payload []byte, meta ...[]byte) []byte {
	mac := hmac.New(t.newHash, key)
	var size [8]byte

	_, _ = mac.Write([]byte{tokenVersion})
	binary.BigEndian.PutUint64(size[:], uint64(len(payload)))
	_, _ = mac.Write(size[:])
	_, _ = mac.Write(payload)
	binary.BigEndian.PutUint64(size[:], uint64(len(meta)))
	_, _ = mac.Write(size[:])
	for _, value := range meta {
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = mac.Write(size[:])
		_, _ = mac.Write(value)
	}
	return mac.Sum(nil)
}

func NewToken(newHash func() hash.Hash) *Token {
	return &Token{newHash: newHash, signatureBytes: newHash().Size()}
}
