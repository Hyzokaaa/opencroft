package entities

import "github.com/Hyzokaaa/opencroft/internal/shared/id"

// User is one person who can operate this host. Pure data — hashing and
// verification live in the domain services.
type User struct {
	Id           id.Id
	Username     string
	PasswordHash string
	Created      string
}

type UserProps struct {
	Id           string
	Username     string
	PasswordHash string
	Created      string
}

func NewUser(props UserProps) *User {
	return &User{
		Id:           id.New(props.Id),
		Username:     props.Username,
		PasswordHash: props.PasswordHash,
		Created:      props.Created,
	}
}

func (u *User) GetId() string {
	return u.Id.Get()
}
