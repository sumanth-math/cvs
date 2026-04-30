package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/your-org/platform-service/internal/api"
	"github.com/your-org/platform-service/internal/config"
	"github.com/your-org/platform-service/internal/health"
	"github.com/your-org/platform-service/internal/provisioner"
	"github.com/your-org/platform-service/internal/workflow"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		logger.Error("failed to load aws configuration", "error", err)
		os.Exit(1)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.Region = cfg.AWSRegion
	})

	bucketProvisioner := provisioner.NewS3BucketProvisioner(s3Client, provisioner.Options{
		Region:       cfg.AWSRegion,
		BucketPrefix: cfg.BucketPrefix,
		DefaultTags:  cfg.DefaultTags,
	})

	var githubClient workflow.GitHubAPI
	if cfg.GitHubToken != "" {
		githubClient = workflow.NewGitHubClient(cfg.GitHubToken, cfg.GitHubAPIURL, nil)
	}

	var snsPublisher workflow.SNSPublisher
	if cfg.DeploymentSNSTopicARN != "" {
		snsPublisher = sns.NewFromConfig(awsCfg, func(options *sns.Options) {
			options.Region = cfg.AWSRegion
		})
	}

	githubWebhooks, err := workflow.NewGitHubWebhookProcessor(workflow.Options{
		GitHub:                    githubClient,
		SNS:                       snsPublisher,
		Logger:                    logger,
		BranchNamePattern:         cfg.GitHubBranchNamePattern,
		AutoLabelPullRequests:     cfg.GitHubAutoLabels,
		DeploymentSummaryTopicARN: cfg.DeploymentSNSTopicARN,
	})
	if err != nil {
		logger.Error("failed to configure github webhook processor", "error", err)
		os.Exit(1)
	}

	healthChecker := health.NewChecker(cfg.HealthCheckTargets, nil)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewServer(bucketProvisioner, logger, api.WithHealthChecker(healthChecker), api.WithGitHubWebhooks(githubWebhooks), api.WithGitHubWebhookSecret(cfg.GitHubWebhookSecret)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("starting platform api", "addr", cfg.HTTPAddr, "region", cfg.AWSRegion)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
