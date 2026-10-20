package monitor

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/safidy/go-maso/apps/api/internal/environment"
	"github.com/safidy/go-maso/apps/api/internal/httpapi"
	"github.com/safidy/go-maso/apps/api/internal/validation"
)

type Scheduler interface {
	Sync(ctx context.Context, monitor Monitor) error
	Remove(ctx context.Context, monitorID string) error
}

type TargetValidator interface {
	ValidateURL(ctx context.Context, value string) error
}

type Handler struct {
	repository            *Repository
	environmentRepository *environment.Repository
	scheduler             Scheduler
	targetValidator       TargetValidator
}

func NewHandler(
	repository *Repository,
	environmentRepository *environment.Repository,
	scheduler Scheduler,
	targetValidator TargetValidator,
) *Handler {
	return &Handler{
		repository:            repository,
		environmentRepository: environmentRepository,
		scheduler:             scheduler,
		targetValidator:       targetValidator,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /environments/{environmentID}/monitors", h.create)
	mux.HandleFunc("GET /environments/{environmentID}/monitors", h.list)
	mux.HandleFunc("GET /monitors/{id}", h.get)
	mux.HandleFunc("PATCH /monitors/{id}", h.update)
	mux.HandleFunc("DELETE /monitors/{id}", h.archive)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")
	if !httpapi.RequireUUID(w, environmentID) {
		return
	}
	if _, err := h.environmentRepository.GetByID(r.Context(), environmentID); errors.Is(err, environment.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "environment not found")
		return
	} else if err != nil {
		h.internalError(w, "get environment", err)
		return
	}

	var input CreateMonitorInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	normalizeCreate(&input)
	if err := validateInput(input.Name, input.URL, input.Method, input.CheckIntervalSeconds, input.TimeoutMS, input.ExpectedStatusMin, input.ExpectedStatusMax, input.FailureThreshold); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.targetValidator.ValidateURL(r.Context(), input.URL); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "monitor target is not allowed: "+err.Error())
		return
	}

	monitor, err := h.repository.Create(r.Context(), environmentID, input)
	if httpapi.IsConflict(err) {
		httpapi.WriteError(w, http.StatusConflict, "an active monitor with this name already exists")
		return
	}
	if err != nil {
		h.internalError(w, "create monitor", err)
		return
	}
	if err := h.scheduler.Sync(r.Context(), monitor); err != nil {
		slog.Error("failed to schedule monitor", "monitor_id", monitor.ID, "error", err)
	}

	httpapi.WriteJSON(w, http.StatusCreated, monitor)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	environmentID := r.PathValue("environmentID")
	if !httpapi.RequireUUID(w, environmentID) {
		return
	}
	if _, err := h.environmentRepository.GetByID(r.Context(), environmentID); errors.Is(err, environment.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "environment not found")
		return
	} else if err != nil {
		h.internalError(w, "get environment", err)
		return
	}

	limit, offset, err := pagination(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	monitors, err := h.repository.ListByEnvironment(r.Context(), environmentID, limit, offset)
	if err != nil {
		h.internalError(w, "list monitors", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, Page{Items: monitors, Limit: limit, Offset: offset})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	monitor, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		h.internalError(w, "get monitor", err)
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, monitor)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	current, err := h.repository.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		h.internalError(w, "get monitor", err)
		return
	}

	var input UpdateMonitorInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	applyUpdate(&current, input)
	if err := validateMonitor(current); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.targetValidator.ValidateURL(r.Context(), current.URL); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "monitor target is not allowed: "+err.Error())
		return
	}

	updated, err := h.repository.Update(r.Context(), current)
	if httpapi.IsConflict(err) {
		httpapi.WriteError(w, http.StatusConflict, "an active monitor with this name already exists")
		return
	}
	if err != nil {
		h.internalError(w, "update monitor", err)
		return
	}
	if err := h.scheduler.Sync(r.Context(), updated); err != nil {
		slog.Error("failed to reschedule monitor", "monitor_id", updated.ID, "error", err)
	}

	httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	if err := h.repository.Archive(r.Context(), id); errors.Is(err, ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return
	} else if err != nil {
		h.internalError(w, "archive monitor", err)
		return
	}

	if err := h.scheduler.Remove(r.Context(), id); err != nil {
		slog.Error("failed to unschedule monitor", "monitor_id", id, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeCreate(input *CreateMonitorInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	if input.CheckIntervalSeconds == 0 {
		input.CheckIntervalSeconds = 30
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 5000
	}
	if input.ExpectedStatusMin == 0 {
		input.ExpectedStatusMin = 200
	}
	if input.ExpectedStatusMax == 0 {
		input.ExpectedStatusMax = 299
	}
	if input.FailureThreshold == 0 {
		input.FailureThreshold = 3
	}
}

func applyUpdate(current *Monitor, input UpdateMonitorInput) {
	if input.Name != nil {
		current.Name = strings.TrimSpace(*input.Name)
	}
	if input.URL != nil {
		current.URL = strings.TrimSpace(*input.URL)
	}
	if input.Method != nil {
		current.Method = strings.ToUpper(strings.TrimSpace(*input.Method))
	}
	if input.CheckIntervalSeconds != nil {
		current.CheckIntervalSeconds = *input.CheckIntervalSeconds
	}
	if input.TimeoutMS != nil {
		current.TimeoutMS = *input.TimeoutMS
	}
	if input.ExpectedStatusMin != nil {
		current.ExpectedStatusMin = *input.ExpectedStatusMin
	}
	if input.ExpectedStatusMax != nil {
		current.ExpectedStatusMax = *input.ExpectedStatusMax
	}
	if input.FailureThreshold != nil {
		current.FailureThreshold = *input.FailureThreshold
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
}

func validateMonitor(value Monitor) error {
	return validateInput(
		value.Name,
		value.URL,
		value.Method,
		value.CheckIntervalSeconds,
		value.TimeoutMS,
		value.ExpectedStatusMin,
		value.ExpectedStatusMax,
		value.FailureThreshold,
	)
}

func validateInput(name, targetURL, method string, intervalSeconds, timeoutMS, statusMin, statusMax, failureThreshold int) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 120 {
		return errors.New("name must contain at most 120 characters")
	}
	if err := validation.HTTPURL(targetURL, true); err != nil {
		return err
	}
	if method != http.MethodGet && method != http.MethodHead {
		return errors.New("method must be GET or HEAD")
	}
	if intervalSeconds < 5 {
		return errors.New("check_interval_seconds must be at least 5")
	}
	if timeoutMS < 100 || timeoutMS > 60000 {
		return errors.New("timeout_ms must be between 100 and 60000")
	}
	if statusMin < 100 || statusMax > 599 || statusMin > statusMax {
		return errors.New("expected status range must be between 100 and 599")
	}
	if failureThreshold < 1 || failureThreshold > 20 {
		return errors.New("failure_threshold must be between 1 and 20")
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
	slog.Error("monitor request failed", "operation", operation, "error", err)
	httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
}
