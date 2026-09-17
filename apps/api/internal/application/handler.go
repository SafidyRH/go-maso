package application

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Handler struct {
	repository *Repository
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{
		repository: repository,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /applications", h.create)
	mux.HandleFunc("GET /applications", h.list)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input CreateApplicationInput

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if input.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	if input.URL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}

	if input.HealthCheckURL == "" {
		http.Error(w, `{"error":"health_check_url is required"}`, http.StatusBadRequest)
		return
	}

	if input.CheckIntervalSeconds == 0 {
		input.CheckIntervalSeconds = 30
	}

	if input.CheckIntervalSeconds < 5 {
		http.Error(
			w,
			`{"error":"check_interval_seconds must be at least 5"}`,
			http.StatusBadRequest,
		)
		return
	}

	app, err := h.repository.Create(r.Context(), input)
	if err != nil {
		slog.Error("failed to create application", "error", err)

		http.Error(
			w,
			`{"error":"failed to create application"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(app); err != nil {
		slog.Error("failed to encode application", "error", err)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	applications, err := h.repository.List(r.Context())
	if err != nil {
		slog.Error("failed to list applications", "error", err)

		http.Error(
			w,
			`{"error":"failed to list applications"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(applications); err != nil {
		slog.Error("failed to encode applications", "error", err)
	}
}
