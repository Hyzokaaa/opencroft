package services

import (
	"context"
	"errors"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/auth/domain/repositories"
)

var ErrCredentialsRejected = errors.New("wrong username or password")

const SessionLifetime = 7 * 24 * time.Hour

type OpenSessionProps struct {
	Username string
	Password string
}

type OpenSession struct {
	users    repositories.UserRepository
	sessions repositories.SessionRepository
	hasher   PasswordHasher
	tokens   TokenGenerator
}

func NewOpenSession(
	users repositories.UserRepository,
	sessions repositories.SessionRepository,
	hasher PasswordHasher,
	tokens TokenGenerator,
) *OpenSession {
	return &OpenSession{users: users, sessions: sessions, hasher: hasher, tokens: tokens}
}

// Execute returns the token to hand to the client. Only its hash is stored.
func (s *OpenSession) Execute(ctx context.Context, props OpenSessionProps) (string, time.Time, error) {
	user, err := s.users.FindByUsername(ctx, props.Username)
	if err != nil {
		return "", time.Time{}, err
	}

	// Verify against a decoy hash when the user does not exist, so that a
	// missing account and a wrong password take the same time to answer.
	if user == nil {
		s.hasher.Matches(props.Password, decoyHash)
		return "", time.Time{}, ErrCredentialsRejected
	}

	if !s.hasher.Matches(props.Password, user.PasswordHash) {
		return "", time.Time{}, ErrCredentialsRejected
	}

	token, hash, err := s.tokens.Create()
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now().UTC()
	expires := now.Add(SessionLifetime)

	session := entities.NewSession(entities.SessionProps{
		TokenHash: hash,
		UserId:    user.GetId(),
		Expires:   expires,
		Created:   now,
	})

	if err := s.sessions.Create(ctx, session); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// A valid bcrypt hash of a value nobody will guess, used only to spend the same
// time hashing when the username does not exist.
const decoyHash = "$2a$12$eImiTXuWVxfM37uY4JANjQ.eImiTXuWVxfM37uY4JANjOoEMIcVBu"
