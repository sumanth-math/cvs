variable "project_name" {
  type        = string
  description = "Short service name used for AWS resource names."
  default     = "platform-service"
}

variable "environment" {
  type        = string
  description = "Deployment environment name."
  default     = "dev"
}

variable "aws_region" {
  type        = string
  description = "AWS region for the ECS workload and S3 API calls."
  default     = "us-east-1"
}

variable "vpc_id" {
  type        = string
  description = "Existing VPC where the load balancer and ECS service will run. Leave empty to create a small demo VPC."
  default     = ""
}

variable "alb_subnet_ids" {
  type        = list(string)
  description = "Existing subnets for the Application Load Balancer. Leave empty along with vpc_id to create demo public subnets."
  default     = []
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Existing private subnets for ECS Fargate tasks. Leave empty along with vpc_id to reuse demo public subnets."
  default     = []
}

variable "internal_alb" {
  type        = bool
  description = "Whether the Application Load Balancer is internal."
  default     = true
}

variable "allowed_ingress_cidr_blocks" {
  type        = list(string)
  description = "CIDR blocks allowed to reach the ALB on HTTP."
  default     = ["0.0.0.0/0"]
}

variable "image_tag" {
  type        = string
  description = "Container image tag deployed to ECS."
  default     = "latest"
}

variable "container_port" {
  type        = number
  description = "Port exposed by the Go API container."
  default     = 8080
}

variable "task_cpu" {
  type        = number
  description = "Fargate task CPU units."
  default     = 256
}

variable "task_memory" {
  type        = number
  description = "Fargate task memory in MiB."
  default     = 512
}

variable "desired_count" {
  type        = number
  description = "Number of ECS tasks to run."
  default     = 2
}

variable "cpu_architecture" {
  type        = string
  description = "Fargate CPU architecture."
  default     = "X86_64"

  validation {
    condition     = contains(["ARM64", "X86_64"], var.cpu_architecture)
    error_message = "cpu_architecture must be ARM64 or X86_64."
  }
}

variable "managed_bucket_prefix" {
  type        = string
  description = "Globally unique prefix for buckets the API is allowed to create."
}

variable "allowed_kms_key_arns" {
  type        = list(string)
  description = "KMS keys the service may reference when callers request aws:kms bucket encryption."
  default     = []
}

variable "deployment_summary_topic_arn" {
  type        = string
  description = "Optional SNS topic ARN where GitHub deployment_status webhook summaries are published."
  default     = ""
}

variable "enable_api_records" {
  type        = bool
  description = "Create a DynamoDB table and record successful API output/audit records."
  default     = true
}

variable "api_records_table_name" {
  type        = string
  description = "Optional DynamoDB table name for API output/audit records. Defaults to project-environment-api-records."
  default     = ""
}

variable "api_records_point_in_time_recovery" {
  type        = bool
  description = "Enable point-in-time recovery on the API records DynamoDB table."
  default     = true
}

variable "health_check_targets" {
  type = list(object({
    name            = string
    url             = string
    method          = optional(string)
    expectedStatus  = optional(number)
    expected_status = optional(number)
    timeout         = optional(string)
  }))
  description = "HTTP services checked by GET /v1/health-checks."
  default     = []
}

variable "portal_catalog_json" {
  type        = string
  description = "Optional JSON catalog document exposed through the developer portal catalog APIs."
  default     = ""
}

variable "github_api_url" {
  type        = string
  description = "GitHub API base URL used by the webhook workflow client."
  default     = "https://api.github.com"
}

variable "github_auto_labels" {
  type        = bool
  description = "Whether pull_request webhook events should auto-apply labels."
  default     = true
}

variable "github_branch_name_pattern" {
  type        = string
  description = "Regular expression enforced against pull request source branch names."
  default     = "^(feature|feat|fix|bugfix|hotfix|chore|docs|refactor|test|ci|build|release|dependabot)/[a-z0-9._-]+$"
}

variable "github_token_secret_arn" {
  type        = string
  description = "Optional SSM Parameter or Secrets Manager ARN injected as GITHUB_TOKEN for webhook GitHub API actions."
  default     = ""
}

variable "github_token" {
  type        = string
  description = "Optional GitHub token injected directly from GitHub Actions repository secrets. Prefer github_token_secret_arn for production."
  default     = ""
  sensitive   = true
}

variable "github_webhook_secret_arn" {
  type        = string
  description = "Optional SSM Parameter or Secrets Manager ARN injected as GITHUB_WEBHOOK_SECRET for signature verification."
  default     = ""
}

variable "github_webhook_secret" {
  type        = string
  description = "Optional GitHub webhook signing secret injected directly from GitHub Actions repository secrets. Prefer github_webhook_secret_arn for production."
  default     = ""
  sensitive   = true
}

variable "github_secret_kms_key_arns" {
  type        = list(string)
  description = "Optional KMS key ARNs needed to decrypt GitHub token/webhook secret parameters."
  default     = []
}

variable "log_level" {
  type        = string
  description = "Structured application log level."
  default     = "info"

  validation {
    condition     = contains(["debug", "info", "warn", "error"], lower(var.log_level))
    error_message = "log_level must be debug, info, warn, or error."
  }
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch log retention in days."
  default     = 30
}

variable "enable_observability" {
  type        = bool
  description = "Create CloudWatch alarms, SNS alerting resources, and a dashboard for the ECS service."
  default     = true
}

variable "create_observability_sns_topic" {
  type        = bool
  description = "Create an SNS topic for CloudWatch alarm notifications."
  default     = true
}

variable "alarm_notification_topic_arns" {
  type        = list(string)
  description = "Existing SNS topic ARNs that receive CloudWatch alarm and OK notifications."
  default     = []
}

variable "alarm_email_endpoints" {
  type        = list(string)
  description = "Email addresses subscribed to the managed observability SNS topic. Each address must confirm the AWS subscription email."
  default     = []
}

variable "alarm_cpu_threshold" {
  type        = number
  description = "Average ECS CPU utilization percent that triggers an alarm."
  default     = 80
}

variable "alarm_memory_threshold" {
  type        = number
  description = "Average ECS memory utilization percent that triggers an alarm."
  default     = 80
}

variable "alarm_5xx_threshold" {
  type        = number
  description = "ALB target 5xx responses per minute that trigger an alarm."
  default     = 5
}

variable "alarm_unhealthy_host_threshold" {
  type        = number
  description = "Average unhealthy ALB target count that triggers an alarm."
  default     = 1
}

variable "alarm_target_response_time_seconds" {
  type        = number
  description = "Average ALB target response time in seconds that triggers an alarm."
  default     = 2
}

variable "enable_execute_command" {
  type        = bool
  description = "Enable ECS Exec for break-glass debugging."
  default     = false
}

variable "tags" {
  type        = map(string)
  description = "Additional tags applied to AWS resources."
  default     = {}
}

variable "github_repository" {
  type        = string
  description = "Optional GitHub repository in owner/name form for an OIDC deploy role."
  default     = ""
}

variable "github_branch" {
  type        = string
  description = "Git branch allowed to assume the optional GitHub Actions deploy role."
  default     = "main"
}

variable "create_github_oidc_provider" {
  type        = bool
  description = "Create the GitHub Actions OIDC provider. Set false if the account already has one."
  default     = false
}

variable "github_oidc_provider_arn" {
  type        = string
  description = "Existing GitHub Actions OIDC provider ARN when create_github_oidc_provider is false."
  default     = ""
}
