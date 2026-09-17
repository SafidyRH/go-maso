package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/application"
)

type Checker struct {
	client *http.Client
}

func NewChecker(timeout time.Duration) *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Checker) Check(
	ctx context.Context,
	app application.Application,
) HealthCheck {
	start := time.Now()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		app.HealthCheckURL,
		nil,
	)

	if err != nil {
		return HealthCheck{
			ApplicationID:  app.ID,
			Status:         StatusDown,
			ResponseTimeMS: time.Since(start).Milliseconds(),
			ErrorMessage:   fmt.Sprintf("create request: %v", err),
		}
	}

	response, err := c.client.Do(req)

	responseTime := time.Since(start).Milliseconds()

	if err != nil {
		return HealthCheck{
			ApplicationID:  app.ID,
			Status:         StatusDown,
			ResponseTimeMS: responseTime,
			ErrorMessage:   err.Error(),
		}
	}

	defer response.Body.Close()

	status := StatusDown

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		status = StatusUp
	}

	return HealthCheck{
		ApplicationID:  app.ID,
		Status:         status,
		StatusCode:     response.StatusCode,
		ResponseTimeMS: responseTime,
	}
}
