package business

import (
	"crypto/rand"
	"encoding/hex"
)

type cryptoTokenGenerator struct{}

func (cryptoTokenGenerator) Generate() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// NewCryptoTokenGenerator returns the production cryptographically secure
// generator. A generated token contains 256 bits of entropy encoded as hex.
func NewCryptoTokenGenerator() TokenGenerator { return cryptoTokenGenerator{} }
