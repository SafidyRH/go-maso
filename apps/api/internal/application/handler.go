package application

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
)

type Scheduler interface {
	RefreshApplication(ctx context.Context, applicationID string) error
}

type Handler struct {
	repository *Repository
	scheduler  Scheduler
}

func NewHandler(repository *Repository, scheduler Scheduler) *Handler {
	return &Handler{repository: repository, scheduler: scheduler}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /applications", h.create)
	mux.HandleFunc("GET /applications", h.list)
	mux.HandleFunc("GET /applications/{id}", h.get)
	mux.HandleFunc("PATCH /applications/{id}", h.update)
	mux.HandleFunc("DELETE /applications/{id}", h.archive)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input CreateApplicationInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Owner = strings.TrimSpace(input.Owner)
	if err := validate(input.Name, input.Owner); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	app, err := h.repository.Create(r.Context(), input)
	if err != nil {
		h.internalError(w, "create application", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusCreated, app)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pagination(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	applications, err := h.repository.List(r.Context(), limit, offset)
	if err != nil {
		h.internalError(w, "list applications", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, Page{Items: applications, Limit: limit, Offset: offset})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	app, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		h.internalError(w, "get application", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, app)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	app, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		h.internalError(w, "get application", err)
		return
	}

	var input UpdateApplicationInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if input.Name != nil {
		app.Name = strings.TrimSpace(*input.Name)
	}
	if input.Owner != nil {
		app.Owner = strings.TrimSpace(*input.Owner)
	}
	if err := validate(app.Name, app.Owner); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.repository.Update(r.Context(), app)
	if err != nil {
		h.internalError(w, "update application", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	if err := h.repository.Archive(r.Context(), id); errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		h.internalError(w, "archive application", err)
		return
	}

	if err := h.scheduler.RefreshApplication(r.Context(), id); err != nil {
		slog.Error("failed to refresh archived application monitors", "application_id", id, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func validate(name, owner string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 120 {
		return errors.New("name must contain at most 120 characters")
	}
	if len(owner) > 120 {
		return errors.New("owner must contain at most 120 characters")
	}
	return nil
}

func pagination(r *http.Request) (int, int, error) {
	limit := 50
	offset := 0
	var err error

	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			return 0, 0, errors.New("limit must be between 1 and 200")
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be zero or greater")
		}
	}

	return limit, offset, nil
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	slog.Error("application request failed", "operation", operation, "error", err)
	httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
}
