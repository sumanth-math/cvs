package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/your-org/platform-service/internal/workflow"
)

type Config struct {
	HTTPAddr                string
	AWSRegion               string
	BucketPrefix            string
	DefaultTags             map[string]string
	GitHubWebhookSecret     string
	GitHubToken             string
	GitHubAPIURL            string
	GitHubBranchNamePattern string
	GitHubAutoLabels        bool
	DeploymentSNSTopicARN   string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                getEnv("HTTP_ADDR", ":8080"),
		AWSRegion:               getEnv("AWS_REGION", "us-east-1"),
		BucketPrefix:            getEnv("BUCKET_PREFIX", "platform-dev"),
		GitHubWebhookSecret:     strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		GitHubToken:             strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubAPIURL:            getEnv("GITHUB_API_URL", "https://api.github.com"),
		GitHubBranchNamePattern: getEnv("GITHUB_BRANCH_NAME_PATTERN", workflow.DefaultBranchNamePattern),
		DeploymentSNSTopicARN:   strings.TrimSpace(os.Getenv("DEPLOYMENT_SUMMARY_TOPIC_ARN")),
		DefaultTags: map[string]string{
			"ManagedBy": "platform-service",
		},
	}

	autoLabels, err := parseBool(getEnv("GITHUB_AUTO_LABELS", "true"), "GITHUB_AUTO_LABELS")
	if err != nil {
		return Config{}, err
	}
	cfg.GitHubAutoLabels = autoLabels

	tags, err := parseTags(os.Getenv("DEFAULT_TAGS"))
	if err != nil {
		return Config{}, err
	}
	for key, value := range tags {
		cfg.DefaultTags[key] = value
	}

	if cfg.AWSRegion == "" {
		return Config{}, fmt.Errorf("AWS_REGION must not be empty")
	}
	if cfg.BucketPrefix == "" {
		return Config{}, fmt.Errorf("BUCKET_PREFIX must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseTags(raw string) (map[string]string, error) {
	tags := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tags, nil
	}

	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("DEFAULT_TAGS must be comma-separated key=value pairs")
		}
		tags[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return tags, nil
}

func parseBool(raw, key string) (bool, error) {
	value := strings.TrimSpace(raw)
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}
