package dashboard

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Handler struct {
	repository *Repository
}

func NewHandler(
	repository *Repository,
) *Handler {
	return &Handler{
		repository: repository,
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
) {
	mux.HandleFunc(
		"GET /dashboard",
		h.list,
	)
}

func (h *Handler) list(
	w http.ResponseWriter,
	r *http.Request,
) {
	applications, err := h.repository.List(
		r.Context(),
	)

	if err != nil {
		slog.Error(
			"failed to load dashboard",
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to load dashboard"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(applications); err != nil {
		slog.Error(
			"failed to encode dashboard",
			"error", err,
		)
	}
}
