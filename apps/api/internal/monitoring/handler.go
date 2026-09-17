package monitoring

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(
		"POST /applications/{id}/check",
		h.checkApplication,
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
