# Architecture Diagram

This folder contains the Draw.io source file for the platform service architecture:

- `platform-service-architecture.drawio.xml`

Open the file in [draw.io / diagrams.net](https://app.diagrams.net/) by choosing **File > Open From > Device**, then selecting the XML file.

## What The Diagram Shows

The diagram is split into three areas:

- **GitHub**: source repository, GitHub Actions workflow, and OIDC role assumption into AWS.
- **AWS Account**: Terraform-managed infrastructure, including the Terraform state bucket, ECR, VPC/subnets, Application Load Balancer, ECS Fargate service, task IAM role, S3, DynamoDB, SNS, CloudWatch, and catalog configuration.
- **Runtime Workflows**: development teams calling the API, GitHub sending webhook events, and external health targets used by the aggregate health endpoint.

## Main Flow

Deployment starts when code is pushed or the `Deploy Platform API` workflow is run manually. GitHub Actions runs tests, builds the Docker image, pushes it to Amazon ECR, assumes the AWS deploy role through OIDC, and runs Terraform. Terraform creates or updates the ECS Fargate workload, the ALB, IAM roles, observability resources, DynamoDB audit table, and other supporting infrastructure.

At runtime, development teams call the ALB, which routes traffic to the Go API running in ECS Fargate. The API provisions guarded S3 buckets, records successful provisioning results to DynamoDB, publishes deployment summaries to SNS, writes logs and metrics to CloudWatch, serves catalog metadata, and processes GitHub webhook events.

## Component Details

### GitHub Components

**Repository (`sumanth-math/cvs`)**

The repository stores the Go application code, Dockerfile, Terraform configuration, GitHub Actions workflow, documentation, and architecture diagram. A push to `main` can trigger tests and, when `ENABLE_DEPLOY_ON_PUSH` is enabled, deployment.

**GitHub Actions**

GitHub Actions is the CI/CD runner for the service. The workflow runs `go test ./...`, builds the container image with Docker Buildx, pushes the image to Amazon ECR, prepares Terraform backend configuration, creates the Terraform state bucket when needed, and applies the Terraform stack.

**OIDC Assume Role**

The workflow authenticates to AWS using GitHub OIDC instead of storing long-lived AWS access keys. GitHub requests a short-lived AWS session for the IAM role stored in `AWS_GITHUB_ACTIONS_ROLE_ARN`. That role is the deploy identity used by Terraform and AWS CLI steps in the workflow.

### AWS Deployment Components

**S3 Terraform State Bucket**

This bucket stores Terraform state for the deployed AWS infrastructure. The workflow creates it if it does not exist and enables versioning, AES256 encryption, and S3 public access blocking. Terraform uses this state to track resources across deployments.

**Terraform Apply**

Terraform is the source of truth for the AWS workload. The configuration under `deploy/terraform` creates or updates the networking, load balancer, ECS service, ECR repository, IAM roles and policies, DynamoDB audit table, CloudWatch resources, SNS topics, and optional GitHub OIDC deployment role.

**Amazon ECR**

Amazon Elastic Container Registry stores the built Docker image for the Go API. GitHub Actions pushes each deployment image using the Git commit SHA as the image tag, and the ECS task definition points to that image.

**VPC and Subnets**

The service runs inside a VPC. For a starter deployment, Terraform can create a small VPC with two public subnets. For production-style use, existing `VPC_ID`, `ALB_SUBNET_IDS`, and `PRIVATE_SUBNET_IDS` values can be supplied through GitHub repository variables.

**Application Load Balancer**

The ALB is the public or internal HTTP entry point for the platform API. It receives requests from developers and GitHub webhooks, then forwards traffic to healthy ECS Fargate tasks on the configured container port.

**ECS Fargate Service**

ECS Fargate runs the Go API container without requiring EC2 instance management. The service hosts all API endpoints, including S3 bucket provisioning, GitHub webhook handling, aggregate health checks, catalog APIs, and basic health checks.

**Task IAM Role**

The ECS task role gives the running API scoped AWS permissions. It allows guarded S3 bucket creation/configuration, DynamoDB audit writes, SNS publishing when deployment summaries are enabled, and any configured secret access for GitHub tokens or webhook secrets.

### Runtime Data and Integration Components

**Managed S3 Buckets**

These are the team-owned buckets created by `POST /v1/s3-buckets`. The API applies guardrails such as deterministic naming, default tags, optional versioning, server-side encryption, public access blocking, and bucket-owner-enforced object ownership.

**DynamoDB API Audit Records**

DynamoDB stores records for successful bucket provisioning responses. Each record captures details such as record type, creation timestamp, request ID, team, environment, bucket name, bucket ARN, region, encryption setting, versioning flag, and tags. This gives the platform team a searchable audit trail.

**SNS Topics**

SNS is used for event notifications. The webhook handler can publish GitHub deployment status summaries to a configured SNS topic, and CloudWatch alarms can publish CPU, memory, ALB error, target health, and latency notifications to an observability topic.

**CloudWatch Logs, Alarms, and Dashboard**

CloudWatch collects structured JSON application logs from ECS and provides metrics for ECS and ALB behavior. Terraform creates alarms for CPU, memory, target 5xx responses, unhealthy targets, and latency, plus a dashboard showing service health and recent logs.

**GitHub API**

When a GitHub token is configured, the webhook processor calls the GitHub API to apply pull request labels, create branch naming status checks, and post workflow feedback. This lets repository workflow automation live inside the platform service.

**Portal Catalog JSON**

The catalog configuration is supplied through `PORTAL_CATALOG_JSON`. The API exposes this data through `/v1/catalog`, `/v1/catalog/services`, `/v1/catalog/environments`, and `/v1/catalog/infrastructure`, giving a lightweight developer portal backend for service and infrastructure metadata.

### Runtime Actors

**Development Teams**

Development teams call the platform API instead of directly provisioning cloud resources. Their main workflow is requesting managed S3 buckets through the HTTP API and reading catalog or health information from the service.

**GitHub Webhooks**

GitHub sends webhook events to `/v1/github/webhook`. The service validates the event payload and, when a webhook secret is configured, verifies the `X-Hub-Signature-256` signature before processing events.

**Configured Health Targets**

Health targets are external HTTP endpoints configured through `HEALTH_CHECK_TARGETS`. The API checks these services through `/v1/health-checks` and returns an aggregate status so consumers can see whether important dependencies are healthy.

## Key Design Point

The service centralizes privileged cloud actions behind a platform-owned API. Development teams get a simple self-service interface, while the platform team keeps consistent controls for naming, tagging, encryption, public access blocking, IAM permissions, audit records, and observability.
