package provisioner

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeBucketRequestDefaultsBucketName(t *testing.T) {
	request, err := NormalizeBucketRequest(BucketRequest{
		Team:        "payments",
		Environment: "dev",
	}, "acme-platform")
	if err != nil {
		t.Fatalf("expected request to be valid: %v", err)
	}

	if request.BucketName != "acme-platform-payments-dev" {
		t.Fatalf("unexpected bucket name: %s", request.BucketName)
	}
	if request.Encryption != "AES256" {
		t.Fatalf("unexpected encryption: %s", request.Encryption)
	}
}

func TestNormalizeBucketRequestValidatesFields(t *testing.T) {
	_, err := NormalizeBucketRequest(BucketRequest{
		Team:        "Payments!",
		Environment: "d",
		BucketName:  "192.168.1.1",
		Encryption:  "plain",
	}, "acme")

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	for _, field := range []string{"team", "environment", "bucketName", "encryption"} {
		if validationErr.Fields[field] == "" {
			t.Fatalf("expected validation error for %s: %#v", field, validationErr.Fields)
		}
	}
}

func TestNormalizeBucketRequestRequiresKMSKey(t *testing.T) {
	_, err := NormalizeBucketRequest(BucketRequest{
		Team:        "payments",
		Environment: "prod",
		Encryption:  "aws:kms",
	}, "acme")

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Fields["kmsKeyArn"] == "" {
		t.Fatalf("expected kmsKeyArn error: %#v", validationErr.Fields)
	}
}

func TestNormalizeBucketRequestRequiresManagedPrefix(t *testing.T) {
	_, err := NormalizeBucketRequest(BucketRequest{
		Team:        "payments",
		Environment: "prod",
		BucketName:  "other-company-payments-prod",
	}, "acme-platform")

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Fields["bucketName"] == "" {
		t.Fatalf("expected bucketName error: %#v", validationErr.Fields)
	}
}

func TestDefaultBucketNameTruncatesWithStableHash(t *testing.T) {
	name := DefaultBucketName("acme-platform-services", strings.Repeat("team-", 12), "development")
	if len(name) > 63 {
		t.Fatalf("bucket name is too long: %d", len(name))
	}

	again := DefaultBucketName("acme-platform-services", strings.Repeat("team-", 12), "development")
	if name != again {
		t.Fatalf("expected deterministic name, got %q then %q", name, again)
	}
}
