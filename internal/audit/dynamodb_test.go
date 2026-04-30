package audit

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/your-org/platform-service/internal/provisioner"
)

func TestRecordBucketProvisioned(t *testing.T) {
	client := &fakeDynamoDB{}
	recorder := NewDynamoDBRecorder(client, "platform-records")
	recorder.now = func() time.Time {
		return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	}

	err := recorder.RecordBucketProvisioned(context.Background(),
		provisioner.BucketRequest{Team: "payments", Environment: "dev"},
		provisioner.BucketResult{
			BucketName:        "platform-payments-dev",
			BucketARN:         "arn:aws:s3:::platform-payments-dev",
			Region:            "us-east-1",
			VersioningEnabled: true,
			Encryption:        "AES256",
			Tags: map[string]string{
				"Team":        "payments",
				"Environment": "dev",
			},
		},
		"request-1",
	)
	if err != nil {
		t.Fatalf("record bucket provisioned: %v", err)
	}

	if client.tableName != "platform-records" {
		t.Fatalf("unexpected table name: %q", client.tableName)
	}
	if client.conditionExpression != "attribute_not_exists(record_id)" {
		t.Fatalf("unexpected condition expression: %q", client.conditionExpression)
	}
	if got := stringAttribute(client.item, "record_type"); got != RecordTypeBucketProvisioned {
		t.Fatalf("unexpected record type: %q", got)
	}
	if got := stringAttribute(client.item, "bucket_name"); got != "platform-payments-dev" {
		t.Fatalf("unexpected bucket name: %q", got)
	}
	if got := stringAttribute(client.item, "request_id"); got != "request-1" {
		t.Fatalf("unexpected request id: %q", got)
	}
}

func TestRecordBucketProvisionedNoopsWithoutTable(t *testing.T) {
	client := &fakeDynamoDB{}
	recorder := NewDynamoDBRecorder(client, "")

	err := recorder.RecordBucketProvisioned(context.Background(), provisioner.BucketRequest{}, provisioner.BucketResult{}, "request-1")
	if err != nil {
		t.Fatalf("record without table: %v", err)
	}
	if client.puts != 0 {
		t.Fatalf("expected no writes, got %d", client.puts)
	}
}

type fakeDynamoDB struct {
	puts                int
	tableName           string
	conditionExpression string
	item                map[string]types.AttributeValue
}

func (f *fakeDynamoDB) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.puts++
	f.tableName = *input.TableName
	f.item = input.Item
	if input.ConditionExpression != nil {
		f.conditionExpression = *input.ConditionExpression
	}
	return &dynamodb.PutItemOutput{}, nil
}

func stringAttribute(item map[string]types.AttributeValue, key string) string {
	value, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return value.Value
}
