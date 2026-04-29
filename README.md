# Platform Self-Service API

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

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | Address the API listens on. |
| `AWS_REGION` | `us-east-1` | AWS region for S3 API calls. |
| `BUCKET_PREFIX` | `platform-dev` | Prefix used for generated bucket names. Make this globally unique. |
| `DEFAULT_TAGS` | empty | Comma-separated `key=value` tags added to provisioned buckets. |

## Deployment

Terraform under `deploy/terraform` creates:

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
| `VPC_ID` | variable | `vpc-0123456789abcdef0` |
| `ALB_SUBNET_IDS` | variable | `["subnet-aaa","subnet-bbb"]` |
| `PRIVATE_SUBNET_IDS` | variable | `["subnet-ccc","subnet-ddd"]` |
| `MANAGED_BUCKET_PREFIX` | variable | `my-company-platform-dev` |

Optional variables include `AWS_REGION`, `PROJECT_NAME`, `ENVIRONMENT`, `CONTAINER_PLATFORM`, `CPU_ARCHITECTURE`, `ALLOWED_INGRESS_CIDR_BLOCKS`, `ALLOWED_KMS_KEY_ARNS`, and `TAGS_JSON`. List and map values should be JSON.

## Local Checks

```sh
go test ./...
docker build -t platform-service:local .
```
