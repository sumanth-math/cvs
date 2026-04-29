package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/platform-service/internal/provisioner"
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

type fakeBucketProvisioner struct {
	result provisioner.BucketResult
	err    error
}

func (f *fakeBucketProvisioner) ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error) {
	return f.result, f.err
}
