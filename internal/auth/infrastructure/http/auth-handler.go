// Package http exposes login and the guard that protects everything else.
package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/repositories"
	"github.com/Hyzokaaa/opencroft/internal/auth/domain/services"
)

const CookieName = "croft_session"

type Handler struct {
	users  repositories.UserRepository
	open   *services.OpenSession
	verify *services.VerifySession
	close  *services.CloseSession
}

func NewHandler(
	users repositories.UserRepository,
	open *services.OpenSession,
	verify *services.VerifySession,
	closeSession *services.CloseSession,
) *Handler {
	return &Handler{users: users, open: open, verify: verify, close: closeSession}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/state", h.state)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}

	token, expires, err := h.open.Execute(r.Context(), services.OpenSessionProps{
		Username: body.Username,
		Password: body.Password,
	})
	if err != nil {
		if errors.Is(err, services.ErrCredentialsRejected) {
			// Never say which half was wrong.
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Wrong username or password."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Only when the request actually arrived over TLS: the common setup
		// today is an SSH tunnel to plain http on localhost, and a Secure
		// cookie there would simply never be sent back.
		Secure: overTLS(r),
	})

	writeJSON(w, http.StatusOK, map[string]string{"username": body.Username})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		_ = h.close.Execute(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// state tells the interface what to render before anyone has logged in: a
// login form, or instructions to create the first user.
func (h *Handler) state(w http.ResponseWriter, r *http.Request) {
	total, err := h.users.Count(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	authenticated := false
	if cookie, err := r.Cookie(CookieName); err == nil {
		session, _ := h.verify.Execute(r.Context(), cookie.Value)
		authenticated = session != nil
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hasUsers":      total > 0,
		"authenticated": authenticated,
	})
}

// Guard refuses everything that is not login or a static asset. It fails
// closed: an unknown path is protected, not open.
func (h *Handler) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if !strings.HasPrefix(r.URL.Path, "/api/") {
			// The interface itself is not a secret; the data behind it is.
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(CookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not signed in."})
			return
		}

		session, err := h.verify.Execute(r.Context(), cookie.Value)
		if err != nil || session == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Your session has expired."})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isPublic(path string) bool {
	switch path {
	case "/api/health", "/api/auth/login", "/api/auth/logout", "/api/auth/state":
		return true
	}
	return false
}

func overTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
