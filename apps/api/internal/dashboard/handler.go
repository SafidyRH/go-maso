package dashboard

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
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
	mux.HandleFunc("GET /dashboard/applications/{id}", h.detail)
}

var supportedPeriods = map[int]struct{}{1: {}, 6: {}, 24: {}, 168: {}, 720: {}}

func parsePeriodHours(r *http.Request) (int, error) {
	value := r.URL.Query().Get("period_hours")
	if value == "" {
		return 24, nil
	}

	hours, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("period_hours must be one of 1, 6, 24, 168 or 720")
	}
	if _, supported := supportedPeriods[hours]; !supported {
		return 0, errors.New("period_hours must be one of 1, 6, 24, 168 or 720")
	}
	return hours, nil
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}

	periodHours, err := parsePeriodHours(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	detail, err := h.repository.GetDetail(r.Context(), id, periodHours)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		slog.Error("failed to load application dashboard", "application_id", id, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "failed to load application dashboard")
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, detail)
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
