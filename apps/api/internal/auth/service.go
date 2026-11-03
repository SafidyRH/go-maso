package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type Service struct {
	repository      *Repository
	absoluteTimeout time.Duration
	fallbackHash    string
}

func NewService(repository *Repository, absoluteTimeout time.Duration) (*Service, error) {
	fallbackHash, err := HashPassword("not-a-real-user-password")
	if err != nil {
		return nil, fmt.Errorf("create authentication fallback hash: %w", err)
	}
	return &Service{repository: repository, absoluteTimeout: absoluteTimeout, fallbackHash: fallbackHash}, nil
}

func (s *Service) Login(
	ctx context.Context,
	email, password, remoteAddress, userAgent string,
) (Credentials, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 254 || password == "" || len(password) > 128 {
		return Credentials{}, ErrInvalidCredentials
	}

	user, passwordHash, err := s.repository.FindUserByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		VerifyPassword(s.fallbackHash, password)
		return Credentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	if user.Disabled || !VerifyPassword(passwordHash, password) {
		return Credentials{}, ErrInvalidCredentials
	}

	sessionToken, err := randomToken()
	if err != nil {
		return Credentials{}, err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return Credentials{}, err
	}
	sessionHash := sha256.Sum256([]byte(sessionToken))
	csrfHash := sha256.Sum256([]byte(csrfToken))
	session, err := s.repository.CreateSession(
		ctx,
		user,
		sessionHash[:],
		csrfHash[:],
		remoteAddress,
		userAgent,
		time.Now().UTC().Add(s.absoluteTimeout),
	)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{SessionToken: sessionToken, CSRFToken: csrfToken, Session: session}, nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate authentication token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
