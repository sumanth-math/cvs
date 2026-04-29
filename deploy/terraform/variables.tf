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
  description = "VPC where the load balancer and ECS service will run."
}

variable "alb_subnet_ids" {
  type        = list(string)
  description = "Subnets for the Application Load Balancer."
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Private subnets for ECS Fargate tasks. These subnets need outbound access to ECR and AWS APIs."
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

variable "log_retention_days" {
  type        = number
  description = "CloudWatch log retention in days."
  default     = 30
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
