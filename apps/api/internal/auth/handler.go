package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
)

type Handler struct {
	service       *Service
	repository    *Repository
	middleware    *Middleware
	trustProxy    bool
	loginAttempts *loginLimiter
}

func NewHandler(service *Service, repository *Repository, middleware *Middleware, trustProxy bool) *Handler {
	return &Handler{
		service:       service,
		repository:    repository,
		middleware:    middleware,
		trustProxy:    trustProxy,
		loginAttempts: newLoginLimiter(5, 15*time.Minute),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /auth/me", h.me)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	remoteAddress := h.remoteAddress(r)
	if !h.loginAttempts.Allow(remoteAddress, time.Now()) {
		w.Header().Set("Retry-After", "900")
		httpapi.WriteError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	credentials, err := h.service.Login(r.Context(), input.Email, input.Password, remoteAddress, r.UserAgent())
	if errors.Is(err, ErrInvalidCredentials) {
		h.loginAttempts.Failure(remoteAddress, time.Now())
		httpapi.WriteError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		slog.Error("login failed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.loginAttempts.Success(remoteAddress)
	h.setCookies(w, credentials)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, http.StatusOK, credentials.Session.User)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.middleware.sessionCookieName()); err == nil {
		if err := h.repository.DeleteSession(r.Context(), cookie.Value); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}
	h.middleware.expireCookies(w)
	w.Header().Set("Clear-Site-Data", `"cache", "cookies", "storage"`)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, session.User)
}

func (h *Handler) setCookies(w http.ResponseWriter, credentials Credentials) {
	http.SetCookie(w, &http.Cookie{
		Name: h.middleware.sessionCookieName(), Value: credentials.SessionToken, Path: "/",
		HttpOnly: true, Secure: h.middleware.secureCookie, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: h.middleware.csrfCookieName(), Value: credentials.CSRFToken, Path: "/",
		HttpOnly: false, Secure: h.middleware.secureCookie, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) remoteAddress(r *http.Request) string {
	if h.trustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
			return forwarded
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

type loginAttempt struct {
	count       int
	windowStart time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	entries  map[string]loginAttempt
	maximum  int
	duration time.Duration
}

func newLoginLimiter(maximum int, duration time.Duration) *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginAttempt), maximum: maximum, duration: duration}
}

func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if now.Sub(entry.windowStart) >= l.duration {
		delete(l.entries, key)
		return true
	}
	return entry.count < l.maximum
}

func (l *loginLimiter) Failure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.duration {
		entry = loginAttempt{windowStart: now}
	}
	entry.count++
	l.entries[key] = entry
}

func (l *loginLimiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
