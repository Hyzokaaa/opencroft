package repositories

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
)

// UserRepository is one of the few repositories backed by our own storage
// rather than the operating system: who may operate this host cannot be
// derived from LXD or nginx.
type UserRepository interface {
	Create(ctx context.Context, user *entities.User) error
	FindByUsername(ctx context.Context, username string) (*entities.User, error)
	FindAll(ctx context.Context) ([]*entities.User, error)
	Count(ctx context.Context) (int, error)
	UpdatePassword(ctx context.Context, username, hash string) error
	Delete(ctx context.Context, username string) error
}

type SessionRepository interface {
	Create(ctx context.Context, session *entities.Session) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*entities.Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context) error
}
