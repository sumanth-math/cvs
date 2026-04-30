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
	"strings"
	"testing"
	"time"

	"github.com/your-org/platform-service/internal/catalog"
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

func TestOpenAPIJSON(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/vnd.oai.openapi+json" {
		t.Fatalf("unexpected content type: %q", contentType)
	}

	var spec map[string]any
	if err := json.NewDecoder(response.Body).Decode(&spec); err != nil {
		t.Fatalf("decode openapi document: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %v", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths object, got %T", spec["paths"])
	}
	if _, ok := paths["/v1/s3-buckets"]; !ok {
		t.Fatal("expected /v1/s3-buckets path in openapi document")
	}
	if _, ok := paths["/v1/sns-topics"]; !ok {
		t.Fatal("expected /v1/sns-topics path in openapi document")
	}
	if _, ok := paths["/v1/github/webhook"]; !ok {
		t.Fatal("expected /v1/github/webhook path in openapi document")
	}
}

func TestSwaggerUI(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Fatal("expected Swagger UI bundle reference")
	}
	if !strings.Contains(body, "/openapi.json") {
		t.Fatal("expected Swagger UI to load /openapi.json")
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
	assertErrorCode(t, response, "not_found")
}

func TestMethodNotAllowedReturnsJSONError(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/s3-buckets", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
	if response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("unexpected Allow header: %q", response.Header().Get("Allow"))
	}
	assertErrorCode(t, response, "method_not_allowed")
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
	request.Header.Set("Content-Type", "application/json")
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

func TestCreateBucketRecordsSuccessfulProvision(t *testing.T) {
	expected := provisioner.BucketResult{
		BucketName:        "acme-platform-payments-dev",
		BucketARN:         "arn:aws:s3:::acme-platform-payments-dev",
		Region:            "us-east-1",
		VersioningEnabled: true,
		Encryption:        "AES256",
	}
	recorder := &fakeBucketProvisionRecorder{}
	handler := NewServer(&fakeBucketProvisioner{result: expected}, nil, WithBucketProvisionRecorder(recorder))
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}
	if recorder.calls != 1 {
		t.Fatalf("expected recorder to be called once, got %d", recorder.calls)
	}
	if recorder.request.Team != "payments" || recorder.request.Environment != "dev" {
		t.Fatalf("unexpected recorder request: %+v", recorder.request)
	}
	if recorder.result.BucketName != expected.BucketName {
		t.Fatalf("unexpected recorder result: %+v", recorder.result)
	}
	if recorder.requestID == "" {
		t.Fatal("expected recorder request ID")
	}
	if recorder.requestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("expected request ID %q, got %q", response.Header().Get("X-Request-ID"), recorder.requestID)
	}
}

func TestCreateBucketContinuesWhenRecordWriteFails(t *testing.T) {
	expected := provisioner.BucketResult{
		BucketName: "acme-platform-payments-dev",
		BucketARN:  "arn:aws:s3:::acme-platform-payments-dev",
		Region:     "us-east-1",
		Encryption: "AES256",
	}
	recorder := &fakeBucketProvisionRecorder{err: fmt.Errorf("dynamodb unavailable")}
	handler := NewServer(&fakeBucketProvisioner{result: expected}, nil, WithBucketProvisionRecorder(recorder))
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}
	if recorder.calls != 1 {
		t.Fatalf("expected recorder to be called once, got %d", recorder.calls)
	}
}

func TestCreateBucketValidationError(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{
		err: &provisioner.ValidationError{Fields: map[string]string{"team": "required"}},
	}, nil)
	body := bytes.NewBufferString(`{"environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestCreateBucketRejectsMissingContentType(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", bytes.NewBufferString(`{"team":"payments","environment":"dev"}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status %d, got %d", http.StatusUnsupportedMediaType, response.Code)
	}
	assertErrorCode(t, response, "unsupported_media_type")
}

func TestCreateBucketRejectsUnknownJSONField(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", bytes.NewBufferString(`{"team":"payments","environment":"dev","unexpected":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorCode(t, response, "invalid_json")
}

func TestCreateBucketRejectsTrailingJSON(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", bytes.NewBufferString(`{"team":"payments","environment":"dev"} {"team":"ops","environment":"dev"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorCode(t, response, "invalid_json")
}

func TestCreateBucketRecoversFromPanic(t *testing.T) {
	handler := NewServer(&panicBucketProvisioner{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/s3-buckets", bytes.NewBufferString(`{"team":"payments","environment":"dev"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}
	assertErrorCode(t, response, "internal_error")
}

func TestCreateSNSTopic(t *testing.T) {
	expected := provisioner.SNSTopicResult{
		TopicName:      "acme-platform-payments-dev",
		TopicARN:       "arn:aws:sns:us-east-1:123456789012:acme-platform-payments-dev",
		Region:         "us-east-1",
		KMSMasterKeyID: "alias/aws/sns",
		Tags:           map[string]string{"Team": "payments", "Environment": "dev"},
	}
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithSNSTopicProvisioner(&fakeSNSTopicProvisioner{result: expected}))
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/sns-topics", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	var actual provisioner.SNSTopicResult
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.TopicName != expected.TopicName {
		t.Fatalf("unexpected topic name: %s", actual.TopicName)
	}
}

func TestCreateSNSTopicRecordsSuccessfulProvision(t *testing.T) {
	expected := provisioner.SNSTopicResult{
		TopicName:      "acme-platform-payments-dev",
		TopicARN:       "arn:aws:sns:us-east-1:123456789012:acme-platform-payments-dev",
		Region:         "us-east-1",
		KMSMasterKeyID: "alias/aws/sns",
		Tags:           map[string]string{"Team": "payments", "Environment": "dev"},
	}
	recorder := &fakeSNSTopicProvisionRecorder{}
	handler := NewServer(&fakeBucketProvisioner{}, nil,
		WithSNSTopicProvisioner(&fakeSNSTopicProvisioner{result: expected}),
		WithSNSTopicProvisionRecorder(recorder),
	)
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/sns-topics", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}
	if recorder.calls != 1 {
		t.Fatalf("expected recorder to be called once, got %d", recorder.calls)
	}
	if recorder.request.Team != "payments" || recorder.request.Environment != "dev" {
		t.Fatalf("unexpected recorder request: %+v", recorder.request)
	}
	if recorder.result.TopicName != expected.TopicName {
		t.Fatalf("unexpected recorder result: %+v", recorder.result)
	}
	if recorder.requestID == "" {
		t.Fatal("expected recorder request ID")
	}
}

func TestCreateSNSTopicValidationError(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithSNSTopicProvisioner(&fakeSNSTopicProvisioner{
		err: &provisioner.ValidationError{Fields: map[string]string{"team": "required"}},
	}))
	body := bytes.NewBufferString(`{"environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/sns-topics", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorCode(t, response, "validation_failed")
}

func TestCreateSNSTopicRequiresProvisioner(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil)
	body := bytes.NewBufferString(`{"team":"payments","environment":"dev"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/sns-topics", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
	assertErrorCode(t, response, "provisioner_unavailable")
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

func TestCatalogListsServices(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/services?owner=platform&environment=dev", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var actual struct {
		Services []catalog.Service `json:"services"`
	}
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(actual.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(actual.Services))
	}
	if actual.Services[0].ID != "platform-api" {
		t.Fatalf("unexpected service id: %q", actual.Services[0].ID)
	}
}

func TestCatalogRejectsUnknownQueryParameter(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/services?team=platform", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorField(t, response, "team")
}

func TestCatalogGetsServiceByID(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/services/platform-api", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var actual catalog.Service
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.Name != "Platform API" {
		t.Fatalf("unexpected service name: %q", actual.Name)
	}
}

func TestCatalogRejectsInvalidServiceID(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/services/bad%20id", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorField(t, response, "id")
}

func TestCatalogGetsEnvironmentByID(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments/dev", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var actual catalog.Environment
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.Region != "us-east-1" {
		t.Fatalf("unexpected region: %q", actual.Region)
	}
}

func TestCatalogListsInfrastructure(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/infrastructure?environment=dev&type=alb", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var actual struct {
		Infrastructure []catalog.InfrastructureResource `json:"infrastructure"`
	}
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(actual.Infrastructure) != 1 {
		t.Fatalf("expected 1 infrastructure resource, got %d", len(actual.Infrastructure))
	}
	if actual.Infrastructure[0].ID != "platform-alb" {
		t.Fatalf("unexpected infrastructure id: %q", actual.Infrastructure[0].ID)
	}
}

func TestCatalogReturnsNotFound(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithCatalog(catalog.NewStaticStore(testCatalog())))
	request := httptest.NewRequest(http.MethodGet, "/v1/catalog/services/missing", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
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
	request.Header.Set("Content-Type", "application/json")
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
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "ping")
	request.Header.Set("X-Hub-Signature-256", "sha256=bad")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestGitHubWebhookRejectsInvalidEventHeader(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil, WithGitHubWebhooks(&fakeGitHubWebhookProcessor{}))
	request := httptest.NewRequest(http.MethodPost, "/v1/github/webhook", bytes.NewReader([]byte(`{"zen":"testing"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "bad event")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	assertErrorCode(t, response, "invalid_event")
}

func TestGitHubWebhookRejectsMissingSignature(t *testing.T) {
	handler := NewServer(&fakeBucketProvisioner{}, nil,
		WithGitHubWebhooks(&fakeGitHubWebhookProcessor{}),
		WithGitHubWebhookSecret("secret"),
	)
	request := httptest.NewRequest(http.MethodPost, "/v1/github/webhook", bytes.NewReader([]byte(`{"zen":"testing"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "ping")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
	assertErrorCode(t, response, "missing_signature")
}

type fakeBucketProvisioner struct {
	result provisioner.BucketResult
	err    error
}

func (f *fakeBucketProvisioner) ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error) {
	return f.result, f.err
}

type fakeBucketProvisionRecorder struct {
	calls     int
	request   provisioner.BucketRequest
	result    provisioner.BucketResult
	requestID string
	err       error
}

func (f *fakeBucketProvisionRecorder) RecordBucketProvisioned(_ context.Context, request provisioner.BucketRequest, result provisioner.BucketResult, requestID string) error {
	f.calls++
	f.request = request
	f.result = result
	f.requestID = requestID
	return f.err
}

type panicBucketProvisioner struct{}

func (p *panicBucketProvisioner) ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error) {
	panic("boom")
}

type fakeSNSTopicProvisioner struct {
	result provisioner.SNSTopicResult
	err    error
}

func (f *fakeSNSTopicProvisioner) ProvisionTopic(context.Context, provisioner.SNSTopicRequest) (provisioner.SNSTopicResult, error) {
	return f.result, f.err
}

type fakeSNSTopicProvisionRecorder struct {
	calls     int
	request   provisioner.SNSTopicRequest
	result    provisioner.SNSTopicResult
	requestID string
	err       error
}

func (f *fakeSNSTopicProvisionRecorder) RecordSNSTopicProvisioned(_ context.Context, request provisioner.SNSTopicRequest, result provisioner.SNSTopicResult, requestID string) error {
	f.calls++
	f.request = request
	f.result = result
	f.requestID = requestID
	return f.err
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

func testCatalog() catalog.Catalog {
	return catalog.Catalog{
		Services: []catalog.Service{
			{
				ID:           "platform-api",
				Name:         "Platform API",
				Owner:        "platform",
				Repository:   "https://github.com/sumanth-math/cvs",
				Environments: []string{"dev"},
			},
			{
				ID:           "payments-api",
				Name:         "Payments API",
				Owner:        "payments",
				Environments: []string{"prod"},
			},
		},
		Environments: []catalog.Environment{
			{
				ID:     "dev",
				Name:   "Development",
				Region: "us-east-1",
			},
		},
		Infrastructure: []catalog.InfrastructureResource{
			{
				ID:          "platform-alb",
				Name:        "Platform ALB",
				Type:        "alb",
				Provider:    "aws",
				Environment: "dev",
			},
			{
				ID:          "payments-bucket",
				Name:        "Payments Bucket",
				Type:        "s3-bucket",
				Provider:    "aws",
				Environment: "prod",
			},
		},
	}
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()

	var actual errorResponse
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if actual.Error != code {
		t.Fatalf("expected error code %q, got %q", code, actual.Error)
	}
}

func assertErrorField(t *testing.T, response *httptest.ResponseRecorder, field string) {
	t.Helper()

	var actual errorResponse
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if actual.Fields[field] == "" {
		t.Fatalf("expected error field %q, got %+v", field, actual.Fields)
	}
}
