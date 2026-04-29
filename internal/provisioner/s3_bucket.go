package provisioner

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const (
	defaultEncryption = "AES256"
)

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)
	bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
)

type S3API interface {
	CreateBucket(context.Context, *s3.CreateBucketInput, ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	PutPublicAccessBlock(context.Context, *s3.PutPublicAccessBlockInput, ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
	PutBucketOwnershipControls(context.Context, *s3.PutBucketOwnershipControlsInput, ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error)
	PutBucketEncryption(context.Context, *s3.PutBucketEncryptionInput, ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	PutBucketVersioning(context.Context, *s3.PutBucketVersioningInput, ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
	PutBucketTagging(context.Context, *s3.PutBucketTaggingInput, ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error)
}

type Options struct {
	Region       string
	BucketPrefix string
	DefaultTags  map[string]string
}

type S3BucketProvisioner struct {
	client       S3API
	region       string
	bucketPrefix string
	defaultTags  map[string]string
}

type BucketRequest struct {
	Team             string            `json:"team"`
	Environment      string            `json:"environment"`
	BucketName       string            `json:"bucketName,omitempty"`
	EnableVersioning *bool             `json:"enableVersioning,omitempty"`
	Encryption       string            `json:"encryption,omitempty"`
	KMSKeyARN        string            `json:"kmsKeyArn,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
}

type BucketResult struct {
	BucketName        string            `json:"bucketName"`
	BucketARN         string            `json:"bucketArn"`
	Region            string            `json:"region"`
	VersioningEnabled bool              `json:"versioningEnabled"`
	Encryption        string            `json:"encryption"`
	Tags              map[string]string `json:"tags"`
}

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "request validation failed"
}

func NewS3BucketProvisioner(client S3API, options Options) *S3BucketProvisioner {
	defaultTags := map[string]string{}
	for key, value := range options.DefaultTags {
		defaultTags[key] = value
	}

	return &S3BucketProvisioner{
		client:       client,
		region:       strings.TrimSpace(options.Region),
		bucketPrefix: strings.Trim(strings.ToLower(options.BucketPrefix), "-."),
		defaultTags:  defaultTags,
	}
}

func (p *S3BucketProvisioner) ProvisionBucket(ctx context.Context, request BucketRequest) (BucketResult, error) {
	normalized, err := NormalizeBucketRequest(request, p.bucketPrefix)
	if err != nil {
		return BucketResult{}, err
	}

	versioningEnabled := true
	if normalized.EnableVersioning != nil {
		versioningEnabled = *normalized.EnableVersioning
	}

	if err := p.createBucket(ctx, normalized.BucketName); err != nil {
		return BucketResult{}, err
	}
	if err := p.blockPublicAccess(ctx, normalized.BucketName); err != nil {
		return BucketResult{}, err
	}
	if err := p.enforceBucketOwner(ctx, normalized.BucketName); err != nil {
		return BucketResult{}, err
	}
	if err := p.configureEncryption(ctx, normalized.BucketName, normalized.Encryption, normalized.KMSKeyARN); err != nil {
		return BucketResult{}, err
	}
	if err := p.configureVersioning(ctx, normalized.BucketName, versioningEnabled); err != nil {
		return BucketResult{}, err
	}

	tags := p.tagsFor(normalized)
	if err := p.tagBucket(ctx, normalized.BucketName, tags); err != nil {
		return BucketResult{}, err
	}

	return BucketResult{
		BucketName:        normalized.BucketName,
		BucketARN:         fmt.Sprintf("arn:aws:s3:::%s", normalized.BucketName),
		Region:            p.region,
		VersioningEnabled: versioningEnabled,
		Encryption:        normalized.Encryption,
		Tags:              tags,
	}, nil
}

func NormalizeBucketRequest(request BucketRequest, prefix string) (BucketRequest, error) {
	normalized := request
	cleanPrefix := strings.Trim(strings.ToLower(prefix), "-.")
	normalized.Team = strings.ToLower(strings.TrimSpace(request.Team))
	normalized.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
	normalized.BucketName = strings.ToLower(strings.TrimSpace(request.BucketName))
	normalized.Encryption = strings.TrimSpace(request.Encryption)
	normalized.KMSKeyARN = strings.TrimSpace(request.KMSKeyARN)

	if normalized.Encryption == "" {
		normalized.Encryption = defaultEncryption
	}

	fields := map[string]string{}
	if !slugPattern.MatchString(normalized.Team) {
		fields["team"] = "must be 3-40 characters using lowercase letters, numbers, and hyphens"
	}
	if !slugPattern.MatchString(normalized.Environment) {
		fields["environment"] = "must be 3-40 characters using lowercase letters, numbers, and hyphens"
	}

	switch normalized.Encryption {
	case "AES256":
	case "aws:kms":
		if normalized.KMSKeyARN == "" {
			fields["kmsKeyArn"] = "is required when encryption is aws:kms"
		}
	default:
		fields["encryption"] = "must be AES256 or aws:kms"
	}

	if normalized.BucketName == "" {
		normalized.BucketName = DefaultBucketName(prefix, normalized.Team, normalized.Environment)
	}

	if !validBucketName(normalized.BucketName) {
		fields["bucketName"] = "must be a valid S3 bucket name: 3-63 lowercase letters, numbers, dots, or hyphens"
	}
	if cleanPrefix != "" && !strings.HasPrefix(normalized.BucketName, cleanPrefix+"-") {
		fields["bucketName"] = fmt.Sprintf("must start with the managed prefix %q", cleanPrefix)
	}

	for key, value := range normalized.Tags {
		if strings.TrimSpace(key) == "" {
			fields["tags"] = "tag keys must not be empty"
			break
		}
		if len(key) > 128 || len(value) > 256 {
			fields["tags"] = "tag keys must be <=128 characters and values <=256 characters"
			break
		}
	}

	if len(fields) > 0 {
		return BucketRequest{}, &ValidationError{Fields: fields}
	}

	return normalized, nil
}

func DefaultBucketName(prefix, team, environment string) string {
	base := strings.Trim(strings.ToLower(prefix), "-.")
	parts := []string{base, team, environment}

	var cleanParts []string
	for _, part := range parts {
		if trimmed := strings.Trim(part, "-."); trimmed != "" {
			cleanParts = append(cleanParts, trimmed)
		}
	}

	name := strings.Join(cleanParts, "-")
	if len(name) <= 63 {
		return name
	}

	hash := sha1.Sum([]byte(name))
	suffix := hex.EncodeToString(hash[:])[:8]
	return fmt.Sprintf("%s-%s", strings.TrimRight(name[:54], "-."), suffix)
}

func validBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	if !bucketPattern.MatchString(name) {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return false
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return false
	}
	return true
}

func (p *S3BucketProvisioner) createBucket(ctx context.Context, bucketName string) error {
	input := &s3.CreateBucketInput{Bucket: aws.String(bucketName)}
	if p.region != "" && p.region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(p.region),
		}
	}

	if _, err := p.client.CreateBucket(ctx, input); err != nil && !bucketAlreadyOwnedByUs(err) {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) blockPublicAccess(ctx context.Context, bucketName string) error {
	_, err := p.client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("block public access: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) enforceBucketOwner(ctx context.Context, bucketName string) error {
	_, err := p.client.PutBucketOwnershipControls(ctx, &s3.PutBucketOwnershipControlsInput{
		Bucket: aws.String(bucketName),
		OwnershipControls: &types.OwnershipControls{
			Rules: []types.OwnershipControlsRule{
				{ObjectOwnership: types.ObjectOwnershipBucketOwnerEnforced},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("enforce bucket owner: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) configureEncryption(ctx context.Context, bucketName, encryption, kmsKeyARN string) error {
	rule := types.ServerSideEncryptionRule{
		ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
			SSEAlgorithm: types.ServerSideEncryptionAes256,
		},
	}

	if encryption == "aws:kms" {
		rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm = types.ServerSideEncryptionAwsKms
		rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID = aws.String(kmsKeyARN)
		rule.BucketKeyEnabled = aws.Bool(true)
	}

	_, err := p.client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucketName),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{rule},
		},
	})
	if err != nil {
		return fmt.Errorf("configure encryption: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) configureVersioning(ctx context.Context, bucketName string, enabled bool) error {
	status := types.BucketVersioningStatusSuspended
	if enabled {
		status = types.BucketVersioningStatusEnabled
	}

	_, err := p.client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucketName),
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: status,
		},
	})
	if err != nil {
		return fmt.Errorf("configure versioning: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) tagBucket(ctx context.Context, bucketName string, tags map[string]string) error {
	tagSet := make([]types.Tag, 0, len(tags))
	for _, key := range sortedKeys(tags) {
		tagSet = append(tagSet, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(tags[key]),
		})
	}

	_, err := p.client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucketName),
		Tagging: &types.Tagging{
			TagSet: tagSet,
		},
	})
	if err != nil {
		return fmt.Errorf("tag bucket: %w", err)
	}
	return nil
}

func (p *S3BucketProvisioner) tagsFor(request BucketRequest) map[string]string {
	tags := map[string]string{}
	for key, value := range p.defaultTags {
		tags[key] = value
	}
	for key, value := range request.Tags {
		tags[key] = value
	}

	tags["Team"] = request.Team
	tags["Environment"] = request.Environment
	tags["Service"] = "platform-service"

	return tags
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func bucketAlreadyOwnedByUs(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "BucketAlreadyOwnedByYou"
}
