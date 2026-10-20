package environment

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/safidy/go-maso/apps/api/internal/application"
	"github.com/safidy/go-maso/apps/api/internal/httpapi"
	"github.com/safidy/go-maso/apps/api/internal/validation"
)

type Scheduler interface {
	RefreshEnvironment(ctx context.Context, environmentID string) error
}

type Handler struct {
	repository            *Repository
	applicationRepository *application.Repository
	scheduler             Scheduler
}

func NewHandler(
	repository *Repository,
	applicationRepository *application.Repository,
	scheduler Scheduler,
) *Handler {
	return &Handler{
		repository:            repository,
		applicationRepository: applicationRepository,
		scheduler:             scheduler,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /applications/{applicationID}/environments", h.create)
	mux.HandleFunc("GET /applications/{applicationID}/environments", h.list)
	mux.HandleFunc("GET /environments/{id}", h.get)
	mux.HandleFunc("PATCH /environments/{id}", h.update)
	mux.HandleFunc("DELETE /environments/{id}", h.archive)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !httpapi.RequireUUID(w, applicationID) {
		return
	}
	if _, err := h.applicationRepository.GetByID(r.Context(), applicationID); errors.Is(err, application.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		h.internalError(w, "get application", err)
		return
	}

	var input CreateEnvironmentInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if err := validate(input.Name, input.BaseURL); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	environment, err := h.repository.Create(r.Context(), applicationID, input)
	if httpapi.IsConflict(err) {
		httpapi.WriteError(w, http.StatusConflict, "an active environment with this name already exists")
		return
	}
	if err != nil {
		h.internalError(w, "create environment", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusCreated, environment)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !httpapi.RequireUUID(w, applicationID) {
		return
	}
	if _, err := h.applicationRepository.GetByID(r.Context(), applicationID); errors.Is(err, application.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		h.internalError(w, "get application", err)
		return
	}

	limit, offset, err := pagination(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	environments, err := h.repository.ListByApplication(r.Context(), applicationID, limit, offset)
	if err != nil {
		h.internalError(w, "list environments", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, Page{Items: environments, Limit: limit, Offset: offset})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	environment, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		h.internalError(w, "get environment", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, environment)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	environment, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		h.internalError(w, "get environment", err)
		return
	}

	var input UpdateEnvironmentInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Name != nil {
		environment.Name = strings.TrimSpace(*input.Name)
	}
	if input.BaseURL != nil {
		environment.BaseURL = strings.TrimSpace(*input.BaseURL)
	}
	if input.Enabled != nil {
		environment.Enabled = *input.Enabled
	}
	if err := validate(environment.Name, environment.BaseURL); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.repository.Update(r.Context(), environment)
	if httpapi.IsConflict(err) {
		httpapi.WriteError(w, http.StatusConflict, "an active environment with this name already exists")
		return
	}
	if err != nil {
		h.internalError(w, "update environment", err)
		return
	}
	if err := h.scheduler.RefreshEnvironment(r.Context(), updated.ID); err != nil {
		slog.Error("failed to refresh environment monitors", "environment_id", updated.ID, "error", err)
	}

	httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	if err := h.repository.Archive(r.Context(), id); errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "environment not found")
		return
	} else if err != nil {
		h.internalError(w, "archive environment", err)
		return
	}

	if err := h.scheduler.RefreshEnvironment(r.Context(), id); err != nil {
		slog.Error("failed to refresh archived environment monitors", "environment_id", id, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func validate(name, baseURL string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 120 {
		return errors.New("name must contain at most 120 characters")
	}
	if err := validation.HTTPURL(baseURL, false); err != nil {
		return err
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
	slog.Error("environment request failed", "operation", operation, "error", err)
	httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
}
