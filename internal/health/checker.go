package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"

	defaultMethod         = http.MethodGet
	defaultExpectedStatus = http.StatusOK
	defaultTimeout        = 2 * time.Second
)

type Target struct {
	Name           string
	URL            string
	Method         string
	ExpectedStatus int
	Timeout        time.Duration
}

type TargetConfig struct {
	Name                string `json:"name"`
	URL                 string `json:"url"`
	Method              string `json:"method,omitempty"`
	ExpectedStatus      int    `json:"expectedStatus,omitempty"`
	ExpectedStatusSnake int    `json:"expected_status,omitempty"`
	Timeout             string `json:"timeout,omitempty"`
}

type Checker struct {
	targets []Target
	client  *http.Client
}

type AggregateResult struct {
	Status     string          `json:"status"`
	CheckedAt  time.Time       `json:"checkedAt"`
	DurationMS int64           `json:"durationMs"`
	Services   []ServiceResult `json:"services"`
}

type ServiceResult struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	ExpectedStatus int    `json:"expectedStatus"`
	DurationMS     int64  `json:"durationMs"`
	Error          string `json:"error,omitempty"`
}

func ParseTargets(raw string) ([]Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var configs []TargetConfig
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		return nil, fmt.Errorf("HEALTH_CHECK_TARGETS must be a JSON array: %w", err)
	}

	targets := make([]Target, 0, len(configs))
	for index, config := range configs {
		target, err := normalizeTarget(config)
		if err != nil {
			return nil, fmt.Errorf("HEALTH_CHECK_TARGETS[%d]: %w", index, err)
		}
		targets = append(targets, target)
	}

	return targets, nil
}

func NewChecker(targets []Target, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{}
	}

	normalized := make([]Target, 0, len(targets))
	for _, target := range targets {
		config := TargetConfig{
			Name:           target.Name,
			URL:            target.URL,
			Method:         target.Method,
			ExpectedStatus: target.ExpectedStatus,
		}
		if target.Timeout > 0 {
			config.Timeout = target.Timeout.String()
		}
		normalizedTarget, err := normalizeTarget(config)
		if err != nil {
			continue
		}
		normalized = append(normalized, normalizedTarget)
	}

	return &Checker{
		targets: normalized,
		client:  client,
	}
}

func (c *Checker) CheckHealth(ctx context.Context) AggregateResult {
	started := time.Now()
	result := AggregateResult{
		Status:    StatusHealthy,
		CheckedAt: started.UTC(),
		Services:  make([]ServiceResult, len(c.targets)),
	}

	var wg sync.WaitGroup
	for index, target := range c.targets {
		wg.Add(1)
		go func(index int, target Target) {
			defer wg.Done()
			result.Services[index] = c.checkTarget(ctx, target)
		}(index, target)
	}
	wg.Wait()

	for _, service := range result.Services {
		if service.Status != StatusHealthy {
			result.Status = StatusUnhealthy
			break
		}
	}
	result.DurationMS = time.Since(started).Milliseconds()

	return result
}

func (c *Checker) checkTarget(ctx context.Context, target Target) ServiceResult {
	started := time.Now()
	result := ServiceResult{
		Name:           target.Name,
		URL:            target.URL,
		Status:         StatusUnhealthy,
		ExpectedStatus: target.ExpectedStatus,
	}

	checkCtx, cancel := context.WithTimeout(ctx, target.Timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(checkCtx, target.Method, target.URL, nil)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	request.Header.Set("User-Agent", "platform-service-health-checker")

	response, err := c.client.Do(request)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	result.HTTPStatus = response.StatusCode
	if response.StatusCode == target.ExpectedStatus {
		result.Status = StatusHealthy
	}
	result.DurationMS = time.Since(started).Milliseconds()

	return result
}

func normalizeTarget(config TargetConfig) (Target, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return Target{}, fmt.Errorf("name is required")
	}

	rawURL := strings.TrimSpace(config.URL)
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return Target{}, fmt.Errorf("url must be valid: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return Target{}, fmt.Errorf("url scheme must be http or https")
	}

	method := strings.ToUpper(strings.TrimSpace(config.Method))
	if method == "" {
		method = defaultMethod
	}
	if method != http.MethodGet && method != http.MethodHead {
		return Target{}, fmt.Errorf("method must be GET or HEAD")
	}

	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = config.ExpectedStatusSnake
	}
	if expectedStatus == 0 {
		expectedStatus = defaultExpectedStatus
	}
	if expectedStatus < 100 || expectedStatus > 599 {
		return Target{}, fmt.Errorf("expectedStatus must be a valid HTTP status code")
	}

	timeout := defaultTimeout
	if strings.TrimSpace(config.Timeout) != "" {
		parsedTimeout, err := time.ParseDuration(strings.TrimSpace(config.Timeout))
		if err != nil {
			return Target{}, fmt.Errorf("timeout must be a duration such as 2s: %w", err)
		}
		if parsedTimeout <= 0 {
			return Target{}, fmt.Errorf("timeout must be greater than zero")
		}
		timeout = parsedTimeout
	}

	return Target{
		Name:           name,
		URL:            rawURL,
		Method:         method,
		ExpectedStatus: expectedStatus,
		Timeout:        timeout,
	}, nil
}
