package incident

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/httpapi"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type TargetValidator interface {
	ValidateURL(context.Context, string) error
}

type Handler struct {
	repository        *Repository
	monitorRepository *monitor.Repository
	targetValidator   TargetValidator
}

func NewHandler(repository *Repository, monitorRepository *monitor.Repository, targetValidator TargetValidator) *Handler {
	return &Handler{
		repository:        repository,
		monitorRepository: monitorRepository,
		targetValidator:   targetValidator,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /monitors/{id}/alert-rule", h.getAlertRule)
	mux.HandleFunc("PUT /monitors/{id}/alert-rule", h.putAlertRule)
	mux.HandleFunc("GET /incidents", h.listIncidents)
	mux.HandleFunc("GET /incidents/{id}", h.getIncident)
	mux.HandleFunc("POST /incidents/{id}/acknowledge", h.acknowledgeIncident)
	mux.HandleFunc("GET /incidents/{id}/events", h.listEvents)
	mux.HandleFunc("GET /incidents/{id}/deliveries", h.listDeliveries)
	mux.HandleFunc("POST /monitors/{id}/maintenance-windows", h.createMaintenance)
	mux.HandleFunc("GET /monitors/{id}/maintenance-windows", h.listMaintenances)
	mux.HandleFunc("DELETE /maintenance-windows/{id}", h.cancelMaintenance)
}

func (h *Handler) getAlertRule(w http.ResponseWriter, r *http.Request) {
	monitorID, ok := h.requireMonitor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	rule, err := h.repository.GetAlertRule(r.Context(), monitorID)
	if errors.Is(err, ErrAlertRuleNotFound) {
		httpapi.WriteJSON(w, http.StatusOK, DefaultAlertRule(monitorID))
		return
	}
	if err != nil {
		h.internalError(w, "get alert rule", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, rule)
}

func (h *Handler) putAlertRule(w http.ResponseWriter, r *http.Request) {
	monitorID, ok := h.requireMonitor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var input AlertRuleInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	normalizeAlertRule(&input)
	existingRule, existingErr := h.repository.GetAlertRule(r.Context(), monitorID)
	if existingErr != nil && !errors.Is(existingErr, ErrAlertRuleNotFound) {
		h.internalError(w, "get existing alert rule", existingErr)
		return
	}
	webhookConfigured := existingErr == nil && existingRule.WebhookConfigured
	if err := validateAlertRule(input, webhookConfigured); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.WebhookURL != "" {
		if err := h.targetValidator.ValidateURL(r.Context(), input.WebhookURL); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "webhook target is not allowed: "+err.Error())
			return
		}
	}

	rule, err := h.repository.UpsertAlertRule(r.Context(), monitorID, input)
	if err != nil {
		h.internalError(w, "save alert rule", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, rule)
}

func (h *Handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	filter := IncidentFilter{
		ApplicationID: r.URL.Query().Get("application_id"),
		MonitorID:     r.URL.Query().Get("monitor_id"),
		Status:        Status(strings.ToUpper(r.URL.Query().Get("status"))),
		Limit:         50,
	}
	if filter.ApplicationID != "" && !httpapi.RequireUUID(w, filter.ApplicationID) {
		return
	}
	if filter.MonitorID != "" && !httpapi.RequireUUID(w, filter.MonitorID) {
		return
	}
	if filter.Status != "" && filter.Status != StatusOpen && filter.Status != StatusAcknowledged && filter.Status != StatusResolved {
		httpapi.WriteError(w, http.StatusBadRequest, "status must be OPEN, ACKNOWLEDGED or RESOLVED")
		return
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			httpapi.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		filter.Limit = limit
	}

	items, err := h.repository.ListIncidents(r.Context(), filter)
	if err != nil {
		h.internalError(w, "list incidents", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) getIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	item, err := h.repository.GetIncident(r.Context(), id)
	if errors.Is(err, ErrIncidentNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		h.internalError(w, "get incident", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) acknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	var input struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input.AcknowledgedBy = strings.TrimSpace(input.AcknowledgedBy)
	if input.AcknowledgedBy == "" || len(input.AcknowledgedBy) > 120 {
		httpapi.WriteError(w, http.StatusBadRequest, "acknowledged_by must contain between 1 and 120 characters")
		return
	}

	item, err := h.repository.Acknowledge(r.Context(), id, input.AcknowledgedBy)
	if errors.Is(err, ErrIncidentNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}
	if errors.Is(err, ErrIncidentResolved) {
		httpapi.WriteError(w, http.StatusConflict, "incident is already resolved")
		return
	}
	if err != nil {
		h.internalError(w, "acknowledge incident", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	h.listIncidentChildren(w, r, "events")
}

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	h.listIncidentChildren(w, r, "deliveries")
}

func (h *Handler) listIncidentChildren(w http.ResponseWriter, r *http.Request, resource string) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	if _, err := h.repository.GetIncident(r.Context(), id); errors.Is(err, ErrIncidentNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "incident not found")
		return
	} else if err != nil {
		h.internalError(w, "get incident", err)
		return
	}
	if resource == "events" {
		items, err := h.repository.ListEvents(r.Context(), id)
		if err != nil {
			h.internalError(w, "list incident events", err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	items, err := h.repository.ListDeliveries(r.Context(), id)
	if err != nil {
		h.internalError(w, "list incident deliveries", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) createMaintenance(w http.ResponseWriter, r *http.Request) {
	monitorID, ok := h.requireMonitor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var input CreateMaintenanceInput
	if err := httpapi.DecodeJSON(w, r, &input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateMaintenance(input, time.Now().UTC()); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.repository.CreateMaintenance(r.Context(), monitorID, input)
	if err != nil {
		h.internalError(w, "create maintenance window", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h *Handler) listMaintenances(w http.ResponseWriter, r *http.Request) {
	monitorID, ok := h.requireMonitor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	items, err := h.repository.ListMaintenances(r.Context(), monitorID)
	if err != nil {
		h.internalError(w, "list maintenance windows", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) cancelMaintenance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpapi.RequireUUID(w, id) {
		return
	}
	if err := h.repository.CancelMaintenance(r.Context(), id); errors.Is(err, ErrMaintenanceNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "maintenance window not found")
		return
	} else if err != nil {
		h.internalError(w, "cancel maintenance window", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requireMonitor(w http.ResponseWriter, r *http.Request, id string) (string, bool) {
	if !httpapi.RequireUUID(w, id) {
		return "", false
	}
	if _, err := h.monitorRepository.GetByID(r.Context(), id); errors.Is(err, monitor.ErrNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "monitor not found")
		return "", false
	} else if err != nil {
		h.internalError(w, "get monitor", err)
		return "", false
	}
	return id, true
}

func normalizeAlertRule(input *AlertRuleInput) {
	input.WebhookURL = strings.TrimSpace(input.WebhookURL)
	if input.FailureThreshold == 0 {
		input.FailureThreshold = 3
	}
	if input.FailureWindowSeconds == 0 {
		input.FailureWindowSeconds = 300
	}
	if input.LatencyConsecutiveChecks == 0 {
		input.LatencyConsecutiveChecks = 3
	}
}

func validateAlertRule(input AlertRuleInput, webhookConfigured bool) error {
	if input.Enabled && input.WebhookURL == "" && !webhookConfigured {
		return errors.New("webhook_url is required when alerting is enabled")
	}
	if input.FailureThreshold < 1 || input.FailureThreshold > 20 {
		return errors.New("failure_threshold must be between 1 and 20")
	}
	if input.FailureWindowSeconds < 30 || input.FailureWindowSeconds > 86400 {
		return errors.New("failure_window_seconds must be between 30 and 86400")
	}
	if input.LatencyThresholdMS < 0 || input.LatencyThresholdMS > 600000 {
		return errors.New("latency_threshold_ms must be between 0 and 600000")
	}
	if input.LatencyConsecutiveChecks < 1 || input.LatencyConsecutiveChecks > 20 {
		return errors.New("latency_consecutive_checks must be between 1 and 20")
	}
	if input.CooldownSeconds < 0 || input.CooldownSeconds > 86400 {
		return errors.New("cooldown_seconds must be between 0 and 86400")
	}
	return nil
}

func validateMaintenance(input CreateMaintenanceInput, now time.Time) error {
	if input.Reason == "" || len(input.Reason) > 240 {
		return errors.New("reason must contain between 1 and 240 characters")
	}
	if input.StartsAt.IsZero() || input.EndsAt.IsZero() {
		return errors.New("starts_at and ends_at are required")
	}
	if !input.EndsAt.After(input.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	if !input.EndsAt.After(now) {
		return errors.New("ends_at must be in the future")
	}
	if input.EndsAt.Sub(input.StartsAt) > 30*24*time.Hour {
		return errors.New("maintenance duration must not exceed 30 days")
	}
	return nil
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	slog.Error("incident request failed", "operation", operation, "error", err)
	httpapi.WriteError(w, http.StatusInternalServerError, "internal server error")
}
