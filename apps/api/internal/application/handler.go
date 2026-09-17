package application

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type Handler struct {
	repository *Repository
	scheduler  MonitorScheduler
}

func NewHandler(repository *Repository, scheduler MonitorScheduler) *Handler {
	return &Handler{
		repository: repository,
		scheduler:  scheduler,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /applications", h.create)
	mux.HandleFunc("GET /applications", h.list)

	mux.HandleFunc(
		"PATCH /applications/{id}",
		h.update,
	)

	mux.HandleFunc(
		"DELETE /applications/{id}",
		h.delete,
	)
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

	if err := h.scheduler.Sync(
		r.Context(),
		app,
	); err != nil {
		slog.Error(
			"failed to sync application with scheduler",
			"application_id", app.ID,
			"error", err,
		)
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

func (h *Handler) update(
	w http.ResponseWriter,
	r *http.Request,
) {
	applicationID := r.PathValue("id")

	app, err := h.repository.GetByID(
		r.Context(),
		applicationID,
	)

	if errors.Is(err, ErrNotFound) {
		http.Error(
			w,
			`{"error":"application not found"}`,
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		slog.Error(
			"failed to get application",
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to get application"}`,
			http.StatusInternalServerError,
		)

		return
	}

	var input UpdateApplicationInput

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		http.Error(
			w,
			`{"error":"invalid request body"}`,
			http.StatusBadRequest,
		)
		return
	}

	if input.Name != nil {
		if *input.Name == "" {
			http.Error(
				w,
				`{"error":"name cannot be empty"}`,
				http.StatusBadRequest,
			)
			return
		}

		app.Name = *input.Name
	}

	if input.URL != nil {
		if *input.URL == "" {
			http.Error(
				w,
				`{"error":"url cannot be empty"}`,
				http.StatusBadRequest,
			)
			return
		}

		app.URL = *input.URL
	}

	if input.HealthCheckURL != nil {
		if *input.HealthCheckURL == "" {
			http.Error(
				w,
				`{"error":"health_check_url cannot be empty"}`,
				http.StatusBadRequest,
			)
			return
		}

		app.HealthCheckURL = *input.HealthCheckURL
	}

	if input.CheckIntervalSeconds != nil {
		if *input.CheckIntervalSeconds < 5 {
			http.Error(
				w,
				`{"error":"check_interval_seconds must be at least 5"}`,
				http.StatusBadRequest,
			)
			return
		}

		app.CheckIntervalSeconds =
			*input.CheckIntervalSeconds
	}

	if input.Enabled != nil {
		app.Enabled = *input.Enabled
	}

	updated, err := h.repository.Update(
		r.Context(),
		app,
	)

	if err != nil {
		slog.Error(
			"failed to update application",
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to update application"}`,
			http.StatusInternalServerError,
		)

		return
	}

	if err := h.scheduler.Sync(
		r.Context(),
		updated,
	); err != nil {
		slog.Error(
			"failed to sync updated application",
			"application_id", updated.ID,
			"error", err,
		)
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(updated); err != nil {
		slog.Error(
			"failed to encode application",
			"error", err,
		)
	}
}

func (h *Handler) delete(
	w http.ResponseWriter,
	r *http.Request,
) {
	applicationID := r.PathValue("id")

	err := h.repository.Delete(
		r.Context(),
		applicationID,
	)

	if errors.Is(err, ErrNotFound) {
		http.Error(
			w,
			`{"error":"application not found"}`,
			http.StatusNotFound,
		)

		return
	}

	if err != nil {
		slog.Error(
			"failed to delete application",
			"error", err,
		)

		http.Error(
			w,
			`{"error":"failed to delete application"}`,
			http.StatusInternalServerError,
		)

		return
	}

	if err := h.scheduler.Remove(
		r.Context(),
		applicationID,
	); err != nil {
		slog.Error(
			"failed to remove application from scheduler",
			"application_id", applicationID,
			"error", err,
		)
	}

	w.WriteHeader(http.StatusNoContent)
}
