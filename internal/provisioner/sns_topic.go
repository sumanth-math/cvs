package provisioner

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
)

const defaultSNSKMSMasterKeyID = "alias/aws/sns"

var snsTopicNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,255}$`)

type SNSAPI interface {
	CreateTopic(context.Context, *sns.CreateTopicInput, ...func(*sns.Options)) (*sns.CreateTopicOutput, error)
	TagResource(context.Context, *sns.TagResourceInput, ...func(*sns.Options)) (*sns.TagResourceOutput, error)
}

type SNSTopicProvisioner struct {
	client      SNSAPI
	region      string
	topicPrefix string
	defaultTags map[string]string
}

type SNSTopicRequest struct {
	Team                      string            `json:"team"`
	Environment               string            `json:"environment"`
	TopicName                 string            `json:"topicName,omitempty"`
	DisplayName               string            `json:"displayName,omitempty"`
	FIFOTopic                 bool              `json:"fifoTopic,omitempty"`
	ContentBasedDeduplication bool              `json:"contentBasedDeduplication,omitempty"`
	KMSMasterKeyID            string            `json:"kmsMasterKeyId,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
}

type SNSTopicResult struct {
	TopicName                 string            `json:"topicName"`
	TopicARN                  string            `json:"topicArn"`
	Region                    string            `json:"region"`
	DisplayName               string            `json:"displayName,omitempty"`
	FIFOTopic                 bool              `json:"fifoTopic"`
	ContentBasedDeduplication bool              `json:"contentBasedDeduplication"`
	KMSMasterKeyID            string            `json:"kmsMasterKeyId"`
	Tags                      map[string]string `json:"tags"`
}

func NewSNSTopicProvisioner(client SNSAPI, options Options) *SNSTopicProvisioner {
	defaultTags := map[string]string{}
	for key, value := range options.DefaultTags {
		defaultTags[key] = value
	}

	return &SNSTopicProvisioner{
		client:      client,
		region:      strings.TrimSpace(options.Region),
		topicPrefix: cleanSNSTopicPrefix(options.BucketPrefix),
		defaultTags: defaultTags,
	}
}

func (p *SNSTopicProvisioner) ProvisionTopic(ctx context.Context, request SNSTopicRequest) (SNSTopicResult, error) {
	normalized, err := NormalizeSNSTopicRequest(request, p.topicPrefix)
	if err != nil {
		return SNSTopicResult{}, err
	}

	tags := p.tagsFor(normalized)
	createOutput, err := p.createTopic(ctx, normalized, tags)
	if err != nil {
		return SNSTopicResult{}, err
	}

	topicARN := aws.ToString(createOutput.TopicArn)
	if err := p.tagTopic(ctx, topicARN, tags); err != nil {
		return SNSTopicResult{}, err
	}

	return SNSTopicResult{
		TopicName:                 normalized.TopicName,
		TopicARN:                  topicARN,
		Region:                    p.region,
		DisplayName:               normalized.DisplayName,
		FIFOTopic:                 normalized.FIFOTopic,
		ContentBasedDeduplication: normalized.ContentBasedDeduplication,
		KMSMasterKeyID:            normalized.KMSMasterKeyID,
		Tags:                      tags,
	}, nil
}

func NormalizeSNSTopicRequest(request SNSTopicRequest, prefix string) (SNSTopicRequest, error) {
	normalized := request
	cleanPrefix := cleanSNSTopicPrefix(prefix)
	normalized.Team = strings.ToLower(strings.TrimSpace(request.Team))
	normalized.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
	normalized.TopicName = strings.ToLower(strings.TrimSpace(request.TopicName))
	normalized.DisplayName = strings.TrimSpace(request.DisplayName)
	normalized.KMSMasterKeyID = strings.TrimSpace(request.KMSMasterKeyID)

	if normalized.KMSMasterKeyID == "" {
		normalized.KMSMasterKeyID = defaultSNSKMSMasterKeyID
	}
	if strings.HasSuffix(normalized.TopicName, ".fifo") {
		normalized.FIFOTopic = true
	}
	if normalized.TopicName == "" {
		normalized.TopicName = DefaultSNSTopicName(cleanPrefix, normalized.Team, normalized.Environment, normalized.FIFOTopic)
	} else if normalized.FIFOTopic && !strings.HasSuffix(normalized.TopicName, ".fifo") {
		normalized.TopicName += ".fifo"
	}

	fields := map[string]string{}
	if !slugPattern.MatchString(normalized.Team) {
		fields["team"] = "must be 3-40 characters using lowercase letters, numbers, and hyphens"
	}
	if !slugPattern.MatchString(normalized.Environment) {
		fields["environment"] = "must be 3-40 characters using lowercase letters, numbers, and hyphens"
	}
	if !validSNSTopicName(normalized.TopicName, normalized.FIFOTopic) {
		fields["topicName"] = "must be 1-256 characters using lowercase letters, numbers, underscores, or hyphens; FIFO topics must end with .fifo"
	}
	if cleanPrefix != "" && !strings.HasPrefix(strings.TrimSuffix(normalized.TopicName, ".fifo"), cleanPrefix+"-") {
		fields["topicName"] = fmt.Sprintf("must start with the managed prefix %q", cleanPrefix)
	}
	if normalized.ContentBasedDeduplication && !normalized.FIFOTopic {
		fields["contentBasedDeduplication"] = "can only be enabled for FIFO topics"
	}
	if len(normalized.DisplayName) > 100 || hasControlCharacter(normalized.DisplayName) {
		fields["displayName"] = "must be 100 characters or fewer and must not contain control characters"
	}
	if len(normalized.KMSMasterKeyID) > 2048 || hasControlCharacter(normalized.KMSMasterKeyID) {
		fields["kmsMasterKeyId"] = "must be 2048 characters or fewer and must not contain control characters"
	}
	if len(normalized.Tags) > 45 {
		fields["tags"] = "must include 45 or fewer custom tags"
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
		return SNSTopicRequest{}, &ValidationError{Fields: fields}
	}

	return normalized, nil
}

func DefaultSNSTopicName(prefix, team, environment string, fifo bool) string {
	base := cleanSNSTopicPrefix(prefix)
	parts := []string{base, strings.ToLower(strings.TrimSpace(team)), strings.ToLower(strings.TrimSpace(environment))}

	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.Trim(part, "-_"); trimmed != "" {
			cleanParts = append(cleanParts, trimmed)
		}
	}

	name := strings.Join(cleanParts, "-")
	suffix := ""
	if fifo {
		suffix = ".fifo"
	}
	if len(name)+len(suffix) <= 256 {
		return name + suffix
	}

	hash := sha1.Sum([]byte(name + suffix))
	hashSuffix := "-" + hex.EncodeToString(hash[:])[:8]
	maxBaseLength := 256 - len(suffix) - len(hashSuffix)
	return strings.TrimRight(name[:maxBaseLength], "-_") + hashSuffix + suffix
}

func validSNSTopicName(name string, fifo bool) bool {
	if len(name) < 1 || len(name) > 256 {
		return false
	}
	body := name
	if fifo {
		if !strings.HasSuffix(name, ".fifo") {
			return false
		}
		body = strings.TrimSuffix(name, ".fifo")
	} else if strings.Contains(name, ".") {
		return false
	}
	return snsTopicNamePattern.MatchString(body)
}

func cleanSNSTopicPrefix(prefix string) string {
	var builder strings.Builder
	previousSeparator := false
	for _, r := range strings.ToLower(strings.TrimSpace(prefix)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			previousSeparator = false
		case r == '-' || r == '_' || r == '.':
			if !previousSeparator {
				builder.WriteByte('-')
				previousSeparator = true
			}
		default:
			if !previousSeparator {
				builder.WriteByte('-')
				previousSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func (p *SNSTopicProvisioner) createTopic(ctx context.Context, request SNSTopicRequest, tags map[string]string) (*sns.CreateTopicOutput, error) {
	attributes := map[string]string{
		"KmsMasterKeyId": request.KMSMasterKeyID,
	}
	if request.DisplayName != "" {
		attributes["DisplayName"] = request.DisplayName
	}
	if request.FIFOTopic {
		attributes["FifoTopic"] = "true"
		attributes["ContentBasedDeduplication"] = strconv.FormatBool(request.ContentBasedDeduplication)
	}

	output, err := p.client.CreateTopic(ctx, &sns.CreateTopicInput{
		Name:       aws.String(request.TopicName),
		Attributes: attributes,
		Tags:       snsTags(tags),
	})
	if err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}
	if aws.ToString(output.TopicArn) == "" {
		return nil, fmt.Errorf("create topic: AWS returned an empty topic ARN")
	}
	return output, nil
}

func (p *SNSTopicProvisioner) tagTopic(ctx context.Context, topicARN string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}

	_, err := p.client.TagResource(ctx, &sns.TagResourceInput{
		ResourceArn: aws.String(topicARN),
		Tags:        snsTags(tags),
	})
	if err != nil {
		return fmt.Errorf("tag topic: %w", err)
	}
	return nil
}

func (p *SNSTopicProvisioner) tagsFor(request SNSTopicRequest) map[string]string {
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
	tags["ResourceType"] = "sns-topic"

	return tags
}

func snsTags(tags map[string]string) []types.Tag {
	tagSet := make([]types.Tag, 0, len(tags))
	for _, key := range sortedKeys(tags) {
		tagSet = append(tagSet, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(tags[key]),
		})
	}
	return tagSet
}

func hasControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
