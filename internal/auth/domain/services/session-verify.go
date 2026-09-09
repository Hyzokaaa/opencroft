package services

import (
	"context"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/auth/domain/repositories"
)

type VerifySession struct {
	sessions repositories.SessionRepository
	tokens   TokenGenerator
}

func NewVerifySession(sessions repositories.SessionRepository, tokens TokenGenerator) *VerifySession {
	return &VerifySession{sessions: sessions, tokens: tokens}
}

// Execute returns nil when the token is unknown or expired. An expired session
// is deleted on the way out rather than left to rot.
func (s *VerifySession) Execute(ctx context.Context, token string) (*entities.Session, error) {
	if token == "" {
		return nil, nil
	}

	hash := s.tokens.HashOf(token)
	session, err := s.sessions.FindByTokenHash(ctx, hash)
	if err != nil || session == nil {
		return nil, err
	}

	if session.Expired(time.Now().UTC()) {
		_ = s.sessions.Delete(ctx, hash)
		return nil, nil
	}
	return session, nil
}

type CloseSession struct {
	sessions repositories.SessionRepository
	tokens   TokenGenerator
}

func NewCloseSession(sessions repositories.SessionRepository, tokens TokenGenerator) *CloseSession {
	return &CloseSession{sessions: sessions, tokens: tokens}
}

func (s *CloseSession) Execute(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, s.tokens.HashOf(token))
}
