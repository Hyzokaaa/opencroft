package acme

import (
	"os"
	"path/filepath"
)

// webRootProvider answers the HTTP-01 challenge by writing the token where
// nginx already serves it.
//
// lego can bind a port itself, but nginx owns port 80 on this host and taking
// it away — even briefly — would drop every other site. Writing a file into a
// directory nginx serves has no such cost, and works the same whether or not
// anything else is listening.
type webRootProvider struct {
	root string
}

func newWebRootProvider(root string) *webRootProvider {
	return &webRootProvider{root: root}
}

func (p *webRootProvider) path(token string) string {
	return filepath.Join(p.root, ".well-known", "acme-challenge", token)
}

func (p *webRootProvider) Present(_, token, keyAuth string) error {
	if err := os.MkdirAll(filepath.Dir(p.path(token)), 0o755); err != nil {
		return err
	}
	// World-readable on purpose: nginx runs as another user and has to read it.
	return os.WriteFile(p.path(token), []byte(keyAuth), 0o644)
}

func (p *webRootProvider) CleanUp(_, token, _ string) error {
	// A leftover token is harmless but untidy, and it is proof of a challenge
	// that already happened. Remove it.
	err := os.Remove(p.path(token))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
