package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/safidy/go-maso/apps/api/internal/validation"
)

const maxRequestBodyBytes = 1 << 20

type ErrorResponse struct {
	Error string `json:"error"`
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}

	return nil
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if value == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("failed to encode HTTP response", "error", err)
	}
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}

func IsConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func RequireUUID(w http.ResponseWriter, value string) bool {
	if validation.UUID(value) {
		return true
	}

	WriteError(w, http.StatusBadRequest, "resource id must be a valid UUID")
	return false
}
