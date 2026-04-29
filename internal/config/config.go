package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	AWSRegion    string
	BucketPrefix string
	DefaultTags  map[string]string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		AWSRegion:    getEnv("AWS_REGION", "us-east-1"),
		BucketPrefix: getEnv("BUCKET_PREFIX", "platform-dev"),
		DefaultTags: map[string]string{
			"ManagedBy": "platform-service",
		},
	}

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
