# Platform Self-Service API

Repository: `sumanth-math/cvs`.

This service exposes a small HTTP API that lets development teams request managed cloud resources. The first supported workflow provisions S3 buckets with guardrails:

- deterministic bucket names from a platform prefix, team, and environment
- S3 block-public-access enabled
- bucket-owner-enforced object ownership
- server-side encryption with AES256 or AWS KMS
- optional versioning
- platform and team tags

The API is written in Go, runs on AWS ECS Fargate, and is deployed with Terraform from GitHub Actions.

## API

Start locally with AWS credentials that can create and configure S3 buckets:

```sh
export AWS_REGION=us-east-1
export BUCKET_PREFIX=my-company-platform
go run ./cmd/platform-api
```

Create a bucket:

```sh
curl -sS -X POST http://localhost:8080/v1/s3-buckets \
  -H 'Content-Type: application/json' \
  -d '{
    "team": "payments",
    "environment": "dev",
    "enableVersioning": true,
    "tags": {
      "CostCenter": "payments"
    }
  }'
```

Use KMS encryption:

```json
{
  "team": "payments",
  "environment": "prod",
  "encryption": "aws:kms",
  "kmsKeyArn": "arn:aws:kms:us-east-1:123456789012:key/example",
  "tags": {
    "DataClass": "confidential"
  }
}
```

Health check:

```sh
curl http://localhost:8080/healthz
```

Aggregated dependency health checks:

```sh
export HEALTH_CHECK_TARGETS='[{"name":"github-api","url":"https://api.github.com/meta","expectedStatus":200,"timeout":"2s"}]'
curl http://localhost:8080/v1/health-checks
```

The aggregated endpoint returns `200` when every configured service returns its expected status code and `503` when any service is unhealthy.

Developer portal catalog:

```sh
export PORTAL_CATALOG_JSON='{"services":[{"id":"platform-api","name":"Platform API","owner":"platform","repository":"https://github.com/sumanth-math/cvs","environments":["dev"]}],"environments":[{"id":"dev","name":"Development","region":"us-east-1"}],"infrastructure":[{"id":"platform-alb","name":"Platform ALB","type":"alb","provider":"aws","environment":"dev"}]}'
curl http://localhost:8080/v1/catalog
curl http://localhost:8080/v1/catalog/services
curl http://localhost:8080/v1/catalog/services/platform-api
curl http://localhost:8080/v1/catalog/environments
curl http://localhost:8080/v1/catalog/infrastructure
```

The catalog endpoints are read-only and return empty lists when `PORTAL_CATALOG_JSON` is not configured. Service lists can be filtered with `owner` and `environment`; infrastructure lists can be filtered with `environment` and `type`.

API errors use a consistent JSON shape. Request bodies for `POST` endpoints must use `Content-Type: application/json`; unknown JSON fields, duplicate JSON objects, unsupported query parameters, invalid catalog IDs, and unsupported methods return validation errors instead of plain-text responses.

```json
{
  "error": "validation_failed",
  "message": "request query validation failed",
  "fields": {
    "team": "is not supported"
  }
}
```

GitHub webhook endpoint:

```sh
curl -sS -X POST http://localhost:8080/v1/github/webhook \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Event: ping' \
  -H 'X-GitHub-Delivery: local-test' \
  -d '{"zen":"Keep it logically awesome."}'
```

The webhook handler supports:

- `pull_request`: auto-labels common PR types and creates a `platform/branch-name` commit status for branch naming conventions.
- `deployment_status`: publishes deployment summaries to SNS when `DEPLOYMENT_SUMMARY_TOPIC_ARN` is configured.
- `ping`: acknowledges GitHub webhook setup checks.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | Address the API listens on. |
| `AWS_REGION` | `us-east-1` | AWS region for S3 API calls. |
| `BUCKET_PREFIX` | `platform-dev` | Prefix used for generated bucket names. Make this globally unique. |
| `DEFAULT_TAGS` | empty | Comma-separated `key=value` tags added to provisioned buckets. |
| `GITHUB_WEBHOOK_SECRET` | empty | Optional GitHub webhook secret used to verify `X-Hub-Signature-256`. |
| `GITHUB_TOKEN` | empty | Optional token used to label PRs, post branch convention comments, and create commit statuses. |
| `GITHUB_BRANCH_NAME_PATTERN` | platform default | Regular expression enforced for pull request source branch names. |
| `GITHUB_AUTO_LABELS` | `true` | Whether pull request webhook events should auto-apply labels. |
| `DEPLOYMENT_SUMMARY_TOPIC_ARN` | empty | Optional SNS topic ARN for deployment status summaries. |
| `HEALTH_CHECK_TARGETS` | empty | JSON array of services checked by `GET /v1/health-checks`. |
| `PORTAL_CATALOG_JSON` | empty | JSON catalog document for developer portal service, environment, and infrastructure metadata. |

## Deployment

Terraform under `deploy/terraform` creates:

- a starter VPC with two public subnets when existing networking is not provided
- ECR repository
- ECS cluster, task definition, and Fargate service
- Application Load Balancer and security groups
- CloudWatch logs
- ECS task execution role
- ECS task role with scoped S3 provisioning permissions
- optional GitHub Actions OIDC deployment role

Copy `deploy/terraform/terraform.tfvars.example` to a real tfvars file or configure the same variables in GitHub Actions.

The workflow in `.github/workflows/deploy.yml` runs tests, builds the container, pushes it to ECR, and applies Terraform.

Configure these GitHub repository settings before running it:

| Setting | Type | Example |
| --- | --- | --- |
| `AWS_GITHUB_ACTIONS_ROLE_ARN` | secret | `arn:aws:iam::123456789012:role/platform-service-dev-github-actions` |
| `TF_STATE_BUCKET` | variable | `my-company-terraform-state` |
| `MANAGED_BUCKET_PREFIX` | variable | `my-company-platform-dev` |

If you do not have VPC subnets yet, leave `VPC_ID`, `ALB_SUBNET_IDS`, and `PRIVATE_SUBNET_IDS` unset. Terraform will create a starter VPC with two public subnets, deploy a public ALB, and assign public IPs to ECS tasks. The task security group only allows inbound traffic from the ALB security group.

When using the starter VPC path, the `AWS_GITHUB_ACTIONS_ROLE_ARN` role needs EC2 permissions to create and delete VPC networking resources, including VPCs, subnets, route tables, routes, Internet Gateways, subnet attributes, VPC attributes, security groups, and tags.

For a production-style deployment, configure these optional repository variables instead:

| Setting | Type | Example |
| --- | --- | --- |
| `VPC_ID` | variable | `vpc-0123456789abcdef0` |
| `ALB_SUBNET_IDS` | variable | `["subnet-aaa","subnet-bbb"]` |
| `PRIVATE_SUBNET_IDS` | variable | `["subnet-ccc","subnet-ddd"]` |

By default, pushes run the Go tests only. To deploy from pushes to `main`, set repository variable `ENABLE_DEPLOY_ON_PUSH` to `true`. You can also deploy from the Actions tab with `workflow_dispatch`.

The workflow will create the Terraform state bucket if it does not already exist, then enable versioning, AES256 encryption, and S3 public access blocking. The GitHub Actions AWS role must have S3 permissions for that bucket.

If Terraform reports that backend argument `bucket` or `key` is missing, confirm `TF_STATE_BUCKET` is set exactly to the bucket name, with no quotes or newline. The workflow trims common copy/paste whitespace, creates a temporary `backend.hcl`, and fails early when the value is blank or invalid.

Optional variables include `AWS_REGION`, `PROJECT_NAME`, `ENVIRONMENT`, `CONTAINER_PLATFORM`, `CPU_ARCHITECTURE`, `ALLOWED_INGRESS_CIDR_BLOCKS`, `ALLOWED_KMS_KEY_ARNS`, and `TAGS_JSON`. List and map values should be JSON.

To configure dependency aggregation in GitHub Actions, add repository variable `HEALTH_CHECK_TARGETS` as a JSON array. The repository variable accepts either `expected_status` or `expectedStatus`:

```json
[{"name":"github-api","url":"https://api.github.com/meta","expected_status":200,"timeout":"2s"}]
```

To configure the developer portal backend in GitHub Actions, add repository variable `PORTAL_CATALOG_JSON` as a JSON object:

```json
{"services":[{"id":"platform-api","name":"Platform API","owner":"platform","repository":"https://github.com/sumanth-math/cvs","environments":["dev"]}],"environments":[{"id":"dev","name":"Development","region":"us-east-1"}],"infrastructure":[{"id":"platform-alb","name":"Platform ALB","type":"alb","provider":"aws","environment":"dev"}]}
```

For GitHub webhook automation using GitHub repository secrets, add `PLATFORM_GITHUB_TOKEN` and `PLATFORM_GITHUB_WEBHOOK_SECRET` under GitHub Actions secrets. The workflow injects those values into ECS as `GITHUB_TOKEN` and `GITHUB_WEBHOOK_SECRET`. The webhook secret value must match the secret configured on the GitHub webhook. The token needs permissions to update pull request labels, create issue comments, and create commit statuses.

The ARN-based variables `GITHUB_TOKEN_SECRET_ARN`, `GITHUB_WEBHOOK_SECRET_ARN`, and `GITHUB_SECRET_KMS_KEY_ARNS` are still supported for a stronger production setup using SSM Parameter Store or Secrets Manager. Direct GitHub-secret injection works, but the values can be stored in Terraform state and ECS task definition history.

## Local Checks

```sh
go test ./...
docker build -t platform-service:local .
```
