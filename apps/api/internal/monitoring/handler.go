package monitoring

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /monitors/{id}/check", h.checkMonitor)
	mux.HandleFunc("GET /monitors/{id}/checks", h.listChecks)
	mux.HandleFunc("GET /monitors/{id}/monitoring-summary", h.getSummary)
}

func (h *Handler) checkMonitor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	result, err := h.service.CheckMonitor(r.Context(), id)
	if errors.Is(err, monitor.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		h.internalError(w, "check monitor", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) listChecks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			httpapi.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}

	results, err := h.service.GetHistory(r.Context(), id, limit)
	if errors.Is(err, monitor.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		h.internalError(w, "get monitor history", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, results)
}

func (h *Handler) getSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	summary, err := h.service.GetSummary(r.Context(), id)
	if errors.Is(err, monitor.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		h.internalError(w, "get monitoring summary", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, summary)
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	slog.Error("monitoring request failed", "operation", operation, "error", err)
	httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
}
