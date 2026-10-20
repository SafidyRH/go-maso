package monitoring

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
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
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split target address: %w", err)
		}

		addresses, err := policy.ResolveHost(ctx, host)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, resolved := range addresses {
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
			if err == nil {
				return connection, nil
			}
			lastErr = err
		}

		return nil, fmt.Errorf("connect to target: %w", lastErr)
	}

	client := &http.Client{Transport: transport}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("too many redirects")
		}
		return policy.ValidateURL(request.Context(), request.URL.String())
	}

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
