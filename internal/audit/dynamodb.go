package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/your-org/platform-service/internal/provisioner"
)

const (
	RecordTypeBucketProvisioned   = "s3_bucket_provisioned"
	RecordTypeSNSTopicProvisioned = "sns_topic_provisioned"
)

type DynamoDBAPI interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type DynamoDBRecorder struct {
	client    DynamoDBAPI
	tableName string
	now       func() time.Time
}

type BucketProvisionedRecord struct {
	RecordID          string            `dynamodbav:"record_id" json:"recordId"`
	RecordType        string            `dynamodbav:"record_type" json:"recordType"`
	CreatedAt         string            `dynamodbav:"created_at" json:"createdAt"`
	RequestID         string            `dynamodbav:"request_id,omitempty" json:"requestId,omitempty"`
	Team              string            `dynamodbav:"team" json:"team"`
	Environment       string            `dynamodbav:"environment" json:"environment"`
	BucketName        string            `dynamodbav:"bucket_name" json:"bucketName"`
	BucketARN         string            `dynamodbav:"bucket_arn" json:"bucketArn"`
	Region            string            `dynamodbav:"region" json:"region"`
	VersioningEnabled bool              `dynamodbav:"versioning_enabled" json:"versioningEnabled"`
	Encryption        string            `dynamodbav:"encryption" json:"encryption"`
	Tags              map[string]string `dynamodbav:"tags,omitempty" json:"tags,omitempty"`
}

type SNSTopicProvisionedRecord struct {
	RecordID                  string            `dynamodbav:"record_id" json:"recordId"`
	RecordType                string            `dynamodbav:"record_type" json:"recordType"`
	CreatedAt                 string            `dynamodbav:"created_at" json:"createdAt"`
	RequestID                 string            `dynamodbav:"request_id,omitempty" json:"requestId,omitempty"`
	Team                      string            `dynamodbav:"team" json:"team"`
	Environment               string            `dynamodbav:"environment" json:"environment"`
	TopicName                 string            `dynamodbav:"topic_name" json:"topicName"`
	TopicARN                  string            `dynamodbav:"topic_arn" json:"topicArn"`
	Region                    string            `dynamodbav:"region" json:"region"`
	DisplayName               string            `dynamodbav:"display_name,omitempty" json:"displayName,omitempty"`
	FIFOTopic                 bool              `dynamodbav:"fifo_topic" json:"fifoTopic"`
	ContentBasedDeduplication bool              `dynamodbav:"content_based_deduplication" json:"contentBasedDeduplication"`
	KMSMasterKeyID            string            `dynamodbav:"kms_master_key_id" json:"kmsMasterKeyId"`
	Tags                      map[string]string `dynamodbav:"tags,omitempty" json:"tags,omitempty"`
}

func NewDynamoDBRecorder(client DynamoDBAPI, tableName string) *DynamoDBRecorder {
	return &DynamoDBRecorder{
		client:    client,
		tableName: strings.TrimSpace(tableName),
		now:       time.Now,
	}
}

func (r *DynamoDBRecorder) RecordBucketProvisioned(ctx context.Context, request provisioner.BucketRequest, result provisioner.BucketResult, requestID string) error {
	if r == nil || r.client == nil || r.tableName == "" {
		return nil
	}

	createdAt := r.now().UTC()
	record := BucketProvisionedRecord{
		RecordID:          recordID(RecordTypeBucketProvisioned, result.BucketName, requestID, createdAt),
		RecordType:        RecordTypeBucketProvisioned,
		CreatedAt:         createdAt.Format(time.RFC3339Nano),
		RequestID:         strings.TrimSpace(requestID),
		Team:              valueFromTags(result.Tags, "Team", request.Team),
		Environment:       valueFromTags(result.Tags, "Environment", request.Environment),
		BucketName:        result.BucketName,
		BucketARN:         result.BucketARN,
		Region:            result.Region,
		VersioningEnabled: result.VersioningEnabled,
		Encryption:        result.Encryption,
		Tags:              result.Tags,
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("marshal bucket provision record: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(record_id)"),
	})
	if err != nil {
		return fmt.Errorf("put bucket provision record: %w", err)
	}

	return nil
}

func (r *DynamoDBRecorder) RecordSNSTopicProvisioned(ctx context.Context, request provisioner.SNSTopicRequest, result provisioner.SNSTopicResult, requestID string) error {
	if r == nil || r.client == nil || r.tableName == "" {
		return nil
	}

	createdAt := r.now().UTC()
	record := SNSTopicProvisionedRecord{
		RecordID:                  recordID(RecordTypeSNSTopicProvisioned, result.TopicName, requestID, createdAt),
		RecordType:                RecordTypeSNSTopicProvisioned,
		CreatedAt:                 createdAt.Format(time.RFC3339Nano),
		RequestID:                 strings.TrimSpace(requestID),
		Team:                      valueFromTags(result.Tags, "Team", request.Team),
		Environment:               valueFromTags(result.Tags, "Environment", request.Environment),
		TopicName:                 result.TopicName,
		TopicARN:                  result.TopicARN,
		Region:                    result.Region,
		DisplayName:               result.DisplayName,
		FIFOTopic:                 result.FIFOTopic,
		ContentBasedDeduplication: result.ContentBasedDeduplication,
		KMSMasterKeyID:            result.KMSMasterKeyID,
		Tags:                      result.Tags,
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("marshal sns topic provision record: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(record_id)"),
	})
	if err != nil {
		return fmt.Errorf("put sns topic provision record: %w", err)
	}

	return nil
}

func valueFromTags(tags map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(tags[key]); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func recordID(recordType, bucketName, requestID string, createdAt time.Time) string {
	parts := []string{
		recordType,
		createdAt.UTC().Format("20060102T150405.000000000Z"),
		strings.TrimSpace(bucketName),
	}
	if requestID = strings.TrimSpace(requestID); requestID != "" {
		parts = append(parts, requestID)
	} else {
		parts = append(parts, randomHex(8))
	}
	return strings.Join(parts, "#")
}

func randomHex(bytesLen int) string {
	data := make([]byte, bytesLen)
	if _, err := rand.Read(data); err != nil {
		return "random-unavailable"
	}
	return hex.EncodeToString(data)
}
