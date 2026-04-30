package provisioner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

func TestNormalizeSNSTopicRequestDefaultsTopicName(t *testing.T) {
	request, err := NormalizeSNSTopicRequest(SNSTopicRequest{
		Team:        "payments",
		Environment: "dev",
	}, "acme-platform")
	if err != nil {
		t.Fatalf("expected request to be valid: %v", err)
	}

	if request.TopicName != "acme-platform-payments-dev" {
		t.Fatalf("unexpected topic name: %s", request.TopicName)
	}
	if request.KMSMasterKeyID != defaultSNSKMSMasterKeyID {
		t.Fatalf("unexpected kms key id: %s", request.KMSMasterKeyID)
	}
}

func TestNormalizeSNSTopicRequestDefaultsFIFOName(t *testing.T) {
	request, err := NormalizeSNSTopicRequest(SNSTopicRequest{
		Team:                      "payments",
		Environment:               "prod",
		FIFOTopic:                 true,
		ContentBasedDeduplication: true,
	}, "acme-platform")
	if err != nil {
		t.Fatalf("expected request to be valid: %v", err)
	}

	if request.TopicName != "acme-platform-payments-prod.fifo" {
		t.Fatalf("unexpected topic name: %s", request.TopicName)
	}
	if !request.FIFOTopic {
		t.Fatal("expected fifo topic")
	}
}

func TestNormalizeSNSTopicRequestValidatesFields(t *testing.T) {
	_, err := NormalizeSNSTopicRequest(SNSTopicRequest{
		Team:                      "Payments!",
		Environment:               "d",
		TopicName:                 "other.topic",
		DisplayName:               strings.Repeat("x", 101),
		ContentBasedDeduplication: true,
	}, "acme")

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	for _, field := range []string{"team", "environment", "topicName", "displayName", "contentBasedDeduplication"} {
		if validationErr.Fields[field] == "" {
			t.Fatalf("expected validation error for %s: %#v", field, validationErr.Fields)
		}
	}
}

func TestNormalizeSNSTopicRequestRequiresManagedPrefix(t *testing.T) {
	_, err := NormalizeSNSTopicRequest(SNSTopicRequest{
		Team:        "payments",
		Environment: "prod",
		TopicName:   "other-company-payments-prod",
	}, "acme-platform")

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Fields["topicName"] == "" {
		t.Fatalf("expected topicName error: %#v", validationErr.Fields)
	}
}

func TestProvisionTopicCreatesAndTagsSNSTopic(t *testing.T) {
	client := &fakeSNS{}
	provisioner := NewSNSTopicProvisioner(client, Options{
		Region:       "us-east-1",
		BucketPrefix: "acme-platform",
		DefaultTags:  map[string]string{"ManagedBy": "platform-service"},
	})

	result, err := provisioner.ProvisionTopic(context.Background(), SNSTopicRequest{
		Team:        "payments",
		Environment: "dev",
		Tags:        map[string]string{"CostCenter": "payments"},
	})
	if err != nil {
		t.Fatalf("provision topic: %v", err)
	}

	if result.TopicName != "acme-platform-payments-dev" {
		t.Fatalf("unexpected topic name: %s", result.TopicName)
	}
	if result.TopicARN != "arn:aws:sns:us-east-1:123456789012:acme-platform-payments-dev" {
		t.Fatalf("unexpected topic arn: %s", result.TopicARN)
	}
	if client.createInput.Attributes["KmsMasterKeyId"] != defaultSNSKMSMasterKeyID {
		t.Fatalf("unexpected kms attribute: %s", client.createInput.Attributes["KmsMasterKeyId"])
	}
	if len(client.tagInput.Tags) == 0 {
		t.Fatal("expected tags to be applied")
	}
}

type fakeSNS struct {
	createInput sns.CreateTopicInput
	tagInput    sns.TagResourceInput
}

func (f *fakeSNS) CreateTopic(_ context.Context, input *sns.CreateTopicInput, _ ...func(*sns.Options)) (*sns.CreateTopicOutput, error) {
	f.createInput = *input
	return &sns.CreateTopicOutput{
		TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:" + aws.ToString(input.Name)),
	}, nil
}

func (f *fakeSNS) TagResource(_ context.Context, input *sns.TagResourceInput, _ ...func(*sns.Options)) (*sns.TagResourceOutput, error) {
	f.tagInput = *input
	return &sns.TagResourceOutput{}, nil
}
