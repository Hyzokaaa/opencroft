package services

import (
	"context"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/auth/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

var (
	ErrUsernameRequired = errors.New("a username is required")
	ErrUsernameInvalid  = errors.New("a username may only contain letters, digits, dots, dashes and underscores")
	ErrUsernameTaken    = errors.New("that username already exists")
	ErrPasswordTooShort = errors.New("the password must be at least 12 characters")
)

// Twelve characters, because this guards a machine that can create and destroy
// containers. Length beats composition rules, so there are none.
const MinPasswordLength = 12

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

type CreateUserProps struct {
	Username string
	Password string
}

type CreateUser struct {
	idGenerator id.Generator
	users       repositories.UserRepository
	hasher      PasswordHasher
}

func NewCreateUser(
	idGenerator id.Generator,
	users repositories.UserRepository,
	hasher PasswordHasher,
) *CreateUser {
	return &CreateUser{idGenerator: idGenerator, users: users, hasher: hasher}
}

func (s *CreateUser) Execute(ctx context.Context, props CreateUserProps) (*entities.User, error) {
	if props.Username == "" {
		return nil, ErrUsernameRequired
	}
	if !usernamePattern.MatchString(props.Username) {
		return nil, ErrUsernameInvalid
	}
	if utf8.RuneCountInString(props.Password) < MinPasswordLength {
		return nil, ErrPasswordTooShort
	}

	existing, err := s.users.FindByUsername(ctx, props.Username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameTaken
	}

	hash, err := s.hasher.Hash(props.Password)
	if err != nil {
		return nil, err
	}

	user := entities.NewUser(entities.UserProps{
		Id:           s.idGenerator.Create(),
		Username:     props.Username,
		PasswordHash: hash,
		Created:      time.Now().UTC().Format(time.RFC3339),
	})

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
