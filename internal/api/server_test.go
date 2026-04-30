package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/your-org/platform-service/internal/health"
	"github.com/your-org/platform-service/internal/provisioner"
	"github.com/your-org/platform-service/internal/workflow"
)

func TestHealth(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestCreateBucket(t *testing.T) {
	expected := provisioner.BucketResult{
		BucketName:        "acme-platform-payments-dev",
		BucketARN:         "arn:aws:s3:::acme-platform-payments-dev",
		Region:            "us-east-1",
		VersioningEnabled: true,
		Encryption:        "AES256",
	}
	handler := NewServer(&fakeBucketProvisioner{result: expected}, nil)
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", body)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	var actual provisioner.BucketResult
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.BucketName != expected.BucketName {
		t.Fatalf("unexpected bucket name: %s", actual.BucketName)
	}
}

func TestCreateBucketValidationError(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{
		err: &provisioner.ValidationError{Fields: map[string]string{"team": "required"}},
	}, nil)
	body := bytes.NewBufferString(`{"environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", body)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestHealthChecksHealthy(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithHealthChecker(&fakeHealthChecker{
		result: health.AggregateResult{
			Status:    health.StatusHealthy,
			CheckedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
			Services: []health.ServiceResult{
				{Name: "github", Status: health.StatusHealthy, HTTPStatus: http.StatusOK, ExpectedStatus: http.StatusOK},
			},
		},
	}))
	request := httptest.NewRequest(http.MethodGet, "/v1/health-checks", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var actual health.AggregateResult
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.Status != health.StatusHealthy {
		t.Fatalf("expected healthy status, got %q", actual.Status)
	}
}

func TestHealthChecksUnhealthy(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithHealthChecker(&fakeHealthChecker{
		result: health.AggregateResult{
			Status: health.StatusUnhealthy,
			Services: []health.ServiceResult{
				{Name: "payments", Status: health.StatusUnhealthy, HTTPStatus: http.StatusInternalServerError, ExpectedStatus: http.StatusOK},
			},
		},
	}))
	request := httptest.NewRequest(http.MethodGet, "/v1/health-checks", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
}

func TestGitHubWebhookProcessesSignedPayload(t *testing.T) {
	processor := &fakeGitHubWebhookProcessor{}
	handler := NewServer(&fakeBucketProvisioner{}, nil,
		WithGitHubWebhooks(processor),
		WithGitHubWebhookSecret("secret"),
	)
	body := []byte(`{"zen":"testing"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/github/webhook", bytes.NewReader(body))
	request.Header.Set("X-GitHub-Event", "ping")
	request.Header.Set("X-GitHub-Delivery", "delivery-1")
	request.Header.Set("X-Hub-Signature-256", signGitHubPayload(body, "secret"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}
	if processor.event.Event != "ping" {
		t.Fatalf("expected ping event, got %q", processor.event.Event)
	}
}

func TestGitHubWebhookRejectsInvalidSignature(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil,
		WithGitHubWebhooks(&fakeGitHubWebhookProcessor{}),
		WithGitHubWebhookSecret("secret"),
	)
	request := httptest.NewRequest(http.MethodPost, "/v1/github/webhook", bytes.NewReader([]byte(`{"zen":"testing"}`)))
	request.Header.Set("X-GitHub-Event", "ping")
	request.Header.Set("X-Hub-Signature-256", "sha256=bad")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

type fakeBucketProvisioner struct {
	result provisioner.BucketResult
	err    error
}

func (f *fakeBucketProvisioner) ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error) {
	return f.result, f.err
}

type fakeHealthChecker struct {
	result health.AggregateResult
}

func (f *fakeHealthChecker) CheckHealth(context.Context) health.AggregateResult {
	return f.result
}

type fakeGitHubWebhookProcessor struct {
	event workflow.GitHubWebhookEvent
}

func (f *fakeGitHubWebhookProcessor) ProcessGitHubWebhook(_ context.Context, event workflow.GitHubWebhookEvent) (workflow.GitHubWebhookResult, error) {
	f.event = event
	return workflow.GitHubWebhookResult{
		Event:      event.Event,
		DeliveryID: event.DeliveryID,
		Actions:    []string{"processed"},
	}, nil
}

func signGitHubPayload(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%x", mac.Sum(nil))
}
