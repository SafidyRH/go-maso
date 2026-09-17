package monitoring

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/safidy/go-maso/apps/api/internal/application"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
) {
	mux.HandleFunc(
		"POST /applications/{id}/check",
		h.checkApplication,
	)

	mux.HandleFunc(
		"GET /applications/{id}/checks",
		h.listChecks,
	)

	mux.HandleFunc(
		"GET /applications/{id}/monitoring-summary",
		h.getSummary,
	)
}

func (h *Handler) checkApplication(
	w http.ResponseWriter,
	r *http.Request,
) {
	applicationID := r.PathValue("id")

	result, err := h.service.CheckApplication(
		r.Context(),
		applicationID,
	)

	if errors.Is(err, application.ErrNotFound) {
		http.Error(
			w,
			`{"error":"application not found"}`,
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		slog.Error(
			"failed to check application",
			"application_id",
			applicationID,
			"error",
			err,
		)

		http.Error(
			w,
			`{"error":"failed to check application"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error(
			"failed to encode health check",
			"error",
			err,
		)
	}
}

func (h *Handler) listChecks(
	w http.ResponseWriter,
	r *http.Request,
) {
	applicationID := r.PathValue("id")

	limit := 50

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)

		if err != nil || parsed < 1 || parsed > 200 {
			http.Error(
				w,
				`{"error":"limit must be between 1 and 200"}`,
				http.StatusBadRequest,
			)

			return
		}

		limit = parsed
	}

	checks, err := h.service.GetHistory(
		r.Context(),
		applicationID,
		limit,
	)

	if errors.Is(err, application.ErrNotFound) {
		http.Error(
			w,
			`{"error":"application not found"}`,
			http.StatusNotFound,
		)

		return
	}

	if err != nil {
		slog.Error(
			"failed to get monitoring history",
			"application_id", applicationID,
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to get monitoring history"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(checks); err != nil {
		slog.Error(
			"failed to encode monitoring history",
			"error", err,
		)
	}
}

func (h *Handler) getSummary(
	w http.ResponseWriter,
	r *http.Request,
) {
	applicationID := r.PathValue("id")

	summary, err := h.service.GetSummary(
		r.Context(),
		applicationID,
	)

	if errors.Is(err, application.ErrNotFound) {
		http.Error(
			w,
			`{"error":"application not found"}`,
			http.StatusNotFound,
		)

		return
	}

	if err != nil {
		slog.Error(
			"failed to get monitoring summary",
			"application_id", applicationID,
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to get monitoring summary"}`,
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		slog.Error(
			"failed to encode monitoring summary",
			"error", err,
		)
	}
}
