package services

// PasswordHasher and TokenGenerator are ports: the domain says what it needs,
// infrastructure decides with which algorithm.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Matches(plain, hash string) bool
}

type TokenGenerator interface {
	// Create returns the secret handed to the client and the hash we store.
	Create() (token string, hash string, err error)
	// HashOf recomputes the stored form of a token presented by a client.
	HashOf(token string) string
}
