package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const defaultRequestTimeout = 10 * time.Second

type apiClient struct {
	baseURL string
	http    *http.Client
}

func newAPIClient(t *testing.T) apiClient {
	t.Helper()

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_API_BASE_URL")), "/")
	if baseURL == "" {
		t.Skip("set PLATFORM_API_BASE_URL to run integration tests")
	}
	parsedBaseURL, err := url.ParseRequestURI(baseURL)
	if err != nil {
		t.Fatalf("PLATFORM_API_BASE_URL must be a valid absolute URL: %v", err)
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		t.Fatal("PLATFORM_API_BASE_URL must include a URL scheme and host")
	}

	return apiClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}
}

func (c apiClient) doJSON(t *testing.T, method, path string, body []byte) (*http.Response, []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Request-ID", "integration-test")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return response, responseBody
}

func TestHealthEndpoint(t *testing.T) {
	client := newAPIClient(t)

	response, body := client.doJSON(t, http.MethodGet, "/healthz", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.StatusCode, body)
	}
	if response.Header.Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header")
	}

	var payload struct {
		Status string `json:"status"`
	}
	decodeJSON(t, body, &payload)
	if payload.Status != "ok" {
		t.Fatalf("expected status ok, got %q", payload.Status)
	}
}

func TestOpenAPIDocumentIncludesCorePaths(t *testing.T) {
	client := newAPIClient(t)

	response, body := client.doJSON(t, http.MethodGet, "/openapi.json", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "application/vnd.oai.openapi+json") {
		t.Fatalf("unexpected content type: %q", contentType)
	}

	var document struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	decodeJSON(t, body, &document)

	if document.OpenAPI != "3.0.3" {
		t.Fatalf("expected OpenAPI 3.0.3, got %q", document.OpenAPI)
	}

	for _, path := range []string{"/healthz", "/v1/s3-buckets", "/v1/catalog", "/v1/github/webhook"} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("expected OpenAPI path %s", path)
		}
	}
}

func TestCatalogEndpointsReturnCollections(t *testing.T) {
	client := newAPIClient(t)

	tests := []struct {
		name string
		path string
		key  string
	}{
		{name: "services", path: "/v1/catalog/services", key: "services"},
		{name: "environments", path: "/v1/catalog/environments", key: "environments"},
		{name: "infrastructure", path: "/v1/catalog/infrastructure", key: "infrastructure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, body := client.doJSON(t, http.MethodGet, tt.path, nil)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.StatusCode, body)
			}

			var payload map[string]json.RawMessage
			decodeJSON(t, body, &payload)
			rawCollection, ok := payload[tt.key]
			if !ok {
				t.Fatalf("expected response key %q in %s", tt.key, body)
			}
			if !json.Valid(rawCollection) || len(rawCollection) == 0 || rawCollection[0] != '[' {
				t.Fatalf("expected %q to be a JSON array, got %s", tt.key, rawCollection)
			}
		})
	}
}

func TestS3BucketValidationDoesNotCreateResource(t *testing.T) {
	client := newAPIClient(t)

	response, body := client.doJSON(t, http.MethodPost, "/v1/s3-buckets", []byte(`{}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.StatusCode, body)
	}

	var payload struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeJSON(t, body, &payload)

	if payload.Error != "validation_failed" {
		t.Fatalf("expected validation_failed error, got %q", payload.Error)
	}
	for _, field := range []string{"team", "environment"} {
		if payload.Fields[field] == "" {
			t.Fatalf("expected validation error for %q, got %v", field, payload.Fields)
		}
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	client := newAPIClient(t)

	response, body := client.doJSON(t, http.MethodGet, "/v1/does-not-exist", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.StatusCode, body)
	}

	var payload struct {
		Error string `json:"error"`
	}
	decodeJSON(t, body, &payload)
	if payload.Error != "not_found" {
		t.Fatalf("expected not_found error, got %q", payload.Error)
	}
}

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()

	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode JSON %q: %v", body, err)
	}
}
