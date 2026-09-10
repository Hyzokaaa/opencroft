package services

import (
	"sync"
	"time"
)

// Attempts slows down guessing.
//
// bcrypt makes each guess expensive for us as well as for the attacker, so
// without a limit an exposed panel is both brute-forceable and trivially
// turned into a way to exhaust the machine's CPU.
//
// It is kept in memory on purpose: a restart forgiving old failures is
// acceptable, and a restart is not something an attacker can cause.
type Attempts struct {
	mu     sync.Mutex
	byKey  map[string]*record
	limit  int
	window time.Duration
	lock   time.Duration
}

type record struct {
	failures int
	first    time.Time
	until    time.Time
}

// Ten failures within five minutes buys a fifteen minute lockout. Loose enough
// that somebody mistyping a password twice never notices, tight enough that
// guessing is hopeless.
func NewAttempts() *Attempts {
	return &Attempts{
		byKey:  map[string]*record{},
		limit:  10,
		window: 5 * time.Minute,
		lock:   15 * time.Minute,
	}
}

// Blocked reports whether this key must wait, and for how long.
func (a *Attempts) Blocked(key string) (time.Duration, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	found, ok := a.byKey[key]
	if !ok {
		return 0, false
	}

	remaining := time.Until(found.until)
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

func (a *Attempts) Failed(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	found, ok := a.byKey[key]

	// A slow trickle of failures over hours is somebody with a bad memory,
	// not an attack: the count only accumulates within the window.
	if !ok || now.Sub(found.first) > a.window {
		a.byKey[key] = &record{failures: 1, first: now}
		return
	}

	found.failures++
	if found.failures >= a.limit {
		found.until = now.Add(a.lock)
		found.failures = 0
		found.first = now
	}
}

// Succeeded clears the count: the person proved they are who they said.
func (a *Attempts) Succeeded(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.byKey, key)
}

// Forget drops records nobody is waiting on, so a stream of made-up usernames
// cannot grow the map without bound.
func (a *Attempts) Forget() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for key, found := range a.byKey {
		if now.After(found.until) && now.Sub(found.first) > a.window {
			delete(a.byKey, key)
		}
	}
}
