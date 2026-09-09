// Package crypto implements the hashing ports with well-understood primitives.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// BcryptHasher stores passwords. The cost is deliberately above the library
// default: this guards a machine that can create and destroy containers, and a
// login takes a quarter of a second at most.
type BcryptHasher struct {
	cost int
}

func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: 12}
}

func (h *BcryptHasher) Hash(plain string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	return string(sum), err
}

func (h *BcryptHasher) Matches(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// RandomTokens issues session and API tokens. The token itself is never
// stored; only its SHA-256, so a copy of the database yields no live sessions.
// SHA-256 is right here and bcrypt is not: the token already has 256 bits of
// entropy, so there is nothing to brute-force and no reason to be slow.
type RandomTokens struct{}

func NewRandomTokens() *RandomTokens {
	return &RandomTokens{}
}

func (t *RandomTokens) Create() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, t.HashOf(token), nil
}

func (t *RandomTokens) HashOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
