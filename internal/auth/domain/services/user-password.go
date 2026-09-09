package services

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"unicode/utf8"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/repositories"
)

var ErrUserNotFound = errors.New("no such user")

type ChangePassword struct {
	users  repositories.UserRepository
	hasher PasswordHasher
}

func NewChangePassword(users repositories.UserRepository, hasher PasswordHasher) *ChangePassword {
	return &ChangePassword{users: users, hasher: hasher}
}

func (s *ChangePassword) Execute(ctx context.Context, username, password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	return s.users.UpdatePassword(ctx, user.Username, hash)
}

// GeneratePassword is for unattended installs. A default password is the most
// exploited weakness in self-hosted software, and "change it later" does not
// protect a machine nobody logs into. A random one printed once has nothing to
// guess and still leaves a working account.
//
// Words would be friendlier, but shipping a dictionary to save a paste is a bad
// trade. This is meant to be copied, used once, and replaced.
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func GeneratePassword() (string, error) {
	const length = 24

	out := make([]byte, length)
	limit := big.NewInt(int64(len(passwordAlphabet)))

	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}
