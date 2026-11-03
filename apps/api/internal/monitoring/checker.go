package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

const maxRedirects = 3

type Checker struct {
	client *http.Client
	policy *NetworkPolicy
}

func NewChecker(policy *NetworkPolicy) *Checker {
	client := NewSafeHTTPClient(policy, 60*time.Second)
	return &Checker{client: client, policy: policy}
}

func (c *Checker) Check(ctx context.Context, target monitor.Monitor) CheckResult {
	startedAt := time.Now()
	result := CheckResult{
		MonitorID: target.ID,
		Outcome:   OutcomeDown,
	}

	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(target.TimeoutMS)*time.Millisecond)
	defer cancel()

	if c.policy != nil {
		if err := c.policy.ValidateURL(checkCtx, target.URL); err != nil {
			result.ResponseTimeMS = time.Since(startedAt).Milliseconds()
			result.ErrorMessage = "target rejected: " + err.Error()
			return result
		}
	}

	request, err := http.NewRequestWithContext(checkCtx, target.Method, target.URL, nil)
	if err != nil {
		result.ResponseTimeMS = time.Since(startedAt).Milliseconds()
		result.ErrorMessage = fmt.Sprintf("create request: %v", err)
		return result
	}
	request.Header.Set("User-Agent", "Go-Maso-Monitor/1.0")

	response, err := c.client.Do(request)
	result.ResponseTimeMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.ErrorMessage = err.Error()
		return result
	}
	defer response.Body.Close()

	result.StatusCode = response.StatusCode
	if response.StatusCode >= target.ExpectedStatusMin && response.StatusCode <= target.ExpectedStatusMax {
		result.Outcome = OutcomeUp
	} else {
		result.ErrorMessage = "unexpected HTTP status " + strconv.Itoa(response.StatusCode)
	}

	return result
}
