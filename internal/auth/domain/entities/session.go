package entities

import "time"

// Session is a browser login. The token is the secret handed to the client;
// only its hash is ever stored, so a stolen database yields no live sessions.
type Session struct {
	TokenHash string
	UserId    string
	Expires   time.Time
	Created   time.Time
}

type SessionProps struct {
	TokenHash string
	UserId    string
	Expires   time.Time
	Created   time.Time
}

func NewSession(props SessionProps) *Session {
	return &Session{
		TokenHash: props.TokenHash,
		UserId:    props.UserId,
		Expires:   props.Expires,
		Created:   props.Created,
	}
}

func (s *Session) Expired(now time.Time) bool {
	return now.After(s.Expires)
}
