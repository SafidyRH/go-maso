package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
)

type sessionStore interface {
	FindSession(context.Context, string, time.Duration) (Session, error)
}

type Middleware struct {
	store        sessionStore
	idleTimeout  time.Duration
	secureCookie bool
}

func NewMiddleware(store sessionStore, idleTimeout time.Duration, secureCookie bool) *Middleware {
	return &Middleware{store: store, idleTimeout: idleTimeout, secureCookie: secureCookie}
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCommonSecurityHeaders(w)
		if publicRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(m.sessionCookieName())
		if err != nil || cookie.Value == "" {
			httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		session, err := m.store.FindSession(r.Context(), cookie.Value, m.idleTimeout)
		if err != nil {
			m.expireCookies(w)
			httpapi.WriteError(w, http.StatusUnauthorized, "session expired")
			return
		}

		if !safeMethod(r.Method) && !m.validCSRF(r, session) {
			httpapi.WriteError(w, http.StatusForbidden, "invalid CSRF token")
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, session)))
	})
}

func SessionFromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(contextKey{}).(Session)
	return session, ok
}

func (m *Middleware) validCSRF(r *http.Request, session Session) bool {
	header := r.Header.Get("X-CSRF-Token")
	cookie, err := r.Cookie(m.csrfCookieName())
	if err != nil || header == "" || cookie.Value == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
		return false
	}
	hash := sha256.Sum256([]byte(header))
	if subtle.ConstantTimeCompare(hash[:], session.CSRFTokenHash) != 1 {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func (m *Middleware) sessionCookieName() string {
	if m.secureCookie {
		return "__Host-go_maso_session"
	}
	return "go_maso_session"
}

func (m *Middleware) csrfCookieName() string {
	if m.secureCookie {
		return "__Host-go_maso_csrf"
	}
	return "go_maso_csrf"
}

func (m *Middleware) expireCookies(w http.ResponseWriter) {
	for _, name := range []string{m.sessionCookieName(), m.csrfCookieName()} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == m.sessionCookieName(),
			Secure: m.secureCookie, SameSite: http.SameSiteStrictMode,
		})
	}
}

func publicRequest(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return r.URL.Path == "/" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" ||
			r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/assets/")
	}
	return r.Method == http.MethodPost && r.URL.Path == "/auth/login"
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func setCommonSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
