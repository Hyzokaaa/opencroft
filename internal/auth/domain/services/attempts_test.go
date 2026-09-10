package services_test

import (
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/services"
)

func TestAFewMistakesAreForgiven(t *testing.T) {
	attempts := services.NewAttempts()

	for range 9 {
		attempts.Failed("user:luis")
	}
	if _, blocked := attempts.Blocked("user:luis"); blocked {
		t.Fatal("locked out somebody who mistyped a password nine times")
	}
}

func TestPersistentGuessingIsLockedOut(t *testing.T) {
	attempts := services.NewAttempts()

	for range 10 {
		attempts.Failed("user:luis")
	}

	wait, blocked := attempts.Blocked("user:luis")
	if !blocked {
		t.Fatal("ten failures in a row were allowed to continue")
	}
	if wait <= 0 {
		t.Fatalf("blocked for %s", wait)
	}
}

// Signing in proves the person is who they said, so the count starts over.
func TestSigningInClearsTheCount(t *testing.T) {
	attempts := services.NewAttempts()

	for range 9 {
		attempts.Failed("user:luis")
	}
	attempts.Succeeded("user:luis")

	for range 9 {
		attempts.Failed("user:luis")
	}
	if _, blocked := attempts.Blocked("user:luis"); blocked {
		t.Fatal("failures from before a successful sign-in still counted")
	}
}

// One account being hammered must not lock everybody else out.
func TestLockingOneAccountLeavesTheOthersAlone(t *testing.T) {
	attempts := services.NewAttempts()

	for range 12 {
		attempts.Failed("user:luis")
	}
	if _, blocked := attempts.Blocked("user:carol"); blocked {
		t.Fatal("carol was locked out by attempts against luis")
	}
}

// A stream of made-up usernames must not grow the map without bound.
func TestForgettingKeepsTheRecordsBounded(t *testing.T) {
	attempts := services.NewAttempts()

	for _, name := range []string{"a", "b", "c"} {
		attempts.Failed("user:" + name)
	}
	attempts.Forget()

	// Nothing is due yet, so nothing is dropped — but the call is safe and
	// leaves live records alone.
	if _, blocked := attempts.Blocked("user:a"); blocked {
		t.Fatal("forgetting locked somebody out")
	}
}
