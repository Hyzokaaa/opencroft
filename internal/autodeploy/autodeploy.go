// Package autodeploy is a service deployed again whenever its branch moves.
//
// The repository says so — a webhook, sent the moment somebody pushes — and,
// in case nobody set one up or one went missing, croft asks every few minutes
// too. Either way what follows is an ordinary redeploy: the plan, the
// snapshot, the record in Activity. What is kept here is only what the
// container cannot say: that the service asked for it, the secret a webhook
// has to know, and the commit that failed, so it is not tried over and over.
package autodeploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
)

type Setting struct {
	Container string
	Service   string
	// Secret is what a webhook has to carry, or sign its body with.
	Secret string
	// Failed is the commit whose deployment failed: it is not tried again,
	// the next one is.
	Failed string
}

type Repository interface {
	Find(ctx context.Context, container, service string) (*Setting, error)
	FindAll(ctx context.Context) ([]Setting, error)
	Save(ctx context.Context, s Setting) error
	Remove(ctx context.Context, container, service string) error
}

// NewSecret is long enough that guessing it is not a plan.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Memory backs the demo and the tests.
type Memory struct {
	mu       sync.Mutex
	settings map[string]Setting
}

func NewMemory() *Memory { return &Memory{settings: map[string]Setting{}} }

func key(container, service string) string { return container + "/" + service }

func (m *Memory) Find(_ context.Context, container, service string) (*Setting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.settings[key(container, service)]; ok {
		return &s, nil
	}
	return nil, nil
}

func (m *Memory) FindAll(_ context.Context) ([]Setting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := make([]Setting, 0, len(m.settings))
	for _, s := range m.settings {
		all = append(all, s)
	}
	sort.Slice(all, func(i, j int) bool {
		return key(all[i].Container, all[i].Service) < key(all[j].Container, all[j].Service)
	})
	return all, nil
}

func (m *Memory) Save(_ context.Context, s Setting) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key(s.Container, s.Service)] = s
	return nil
}

func (m *Memory) Remove(_ context.Context, container, service string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.settings, key(container, service))
	return nil
}
