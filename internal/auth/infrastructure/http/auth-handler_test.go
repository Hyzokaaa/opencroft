package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/services"
	"github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/crypto"
	"github.com/Hyzokaaa/opencroft/internal/auth/infrastructure/sqlite"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

// The panel names whoever is signed in, so that on a host with more than
// one user nobody acts in somebody else's session without seeing it. Before
// signing in there is nobody to name, and the answer must not hint at one.
func TestTheStateSaysWhoIsSignedIn(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "croft.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	users := sqlite.NewSQLiteUserRepository(store)
	sessions := sqlite.NewSQLiteSessionRepository(store)
	hasher, tokens := crypto.NewBcryptHasher(), crypto.NewRandomTokens()
	create := services.NewCreateUser(id.NewULIDGenerator(), users, hasher)
	for _, name := range []string{"luis", "maria"} {
		if _, err := create.Execute(context.Background(), services.CreateUserProps{Username: name, Password: "a long enough password"}); err != nil {
			t.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	NewHandler(users, services.NewOpenSession(users, sessions, hasher, tokens),
		services.NewVerifySession(sessions, tokens), services.NewCloseSession(sessions, tokens)).Register(mux)

	state := func(cookies []*http.Cookie) map[string]any {
		r := httptest.NewRequest(http.MethodGet, "/api/auth/state", nil)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	if got := state(nil)["username"]; got != "" {
		t.Errorf("signed out, the state names %q", got)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"maria","password":"a long enough password"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, login)
	if w.Code != http.StatusOK {
		t.Fatalf("signing in answered %d: %s", w.Code, w.Body)
	}

	if got := state(w.Result().Cookies())["username"]; got != "maria" {
		t.Errorf("signed in as maria, the state names %q", got)
	}
}
