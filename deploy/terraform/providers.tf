provider "aws" {
  region = var.aws_region

  default_tags {
    tags = local.common_tags
  }
}

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  name                 = "${var.project_name}-${var.environment}"
  container_name       = "platform-api"
  network_inputs_empty = trimspace(var.vpc_id) == "" && length(var.alb_subnet_ids) == 0 && length(var.private_subnet_ids) == 0
  create_network       = local.network_inputs_empty

  vpc_id             = local.create_network ? aws_vpc.managed[0].id : trimspace(var.vpc_id)
  alb_subnet_ids     = local.create_network ? aws_subnet.public[*].id : var.alb_subnet_ids
  service_subnet_ids = local.create_network ? aws_subnet.public[*].id : var.private_subnet_ids
  assign_public_ip   = local.create_network
  internal_alb       = local.create_network ? false : var.internal_alb
  github_secret_arns = compact([var.github_token_secret_arn, var.github_webhook_secret_arn])
  health_check_targets = [
    for target in var.health_check_targets : merge(
      {
        name = target.name
        url  = target.url
      },
      target.method != null ? { method = target.method } : {},
      target.expected_status != null ? { expectedStatus = target.expected_status } : {},
      target.expectedStatus != null ? { expectedStatus = target.expectedStatus } : {},
      target.timeout != null ? { timeout = target.timeout } : {}
    )
  ]
  container_environment = concat(
    [
      {
        name  = "AWS_REGION"
        value = var.aws_region
      },
      {
        name  = "BUCKET_PREFIX"
        value = var.managed_bucket_prefix
      },
      {
        name  = "HTTP_ADDR"
        value = ":${var.container_port}"
      },
      {
        name  = "DEFAULT_TAGS"
        value = "Environment=${var.environment},Project=${var.project_name},ManagedBy=platform-service"
      },
      {
        name  = "LOG_LEVEL"
        value = lower(var.log_level)
      },
      {
        name  = "GITHUB_API_URL"
        value = var.github_api_url
      },
      {
        name  = "GITHUB_AUTO_LABELS"
        value = tostring(var.github_auto_labels)
      },
      {
        name  = "GITHUB_BRANCH_NAME_PATTERN"
        value = var.github_branch_name_pattern
      },
      {
        name  = "DEPLOYMENT_SUMMARY_TOPIC_ARN"
        value = var.deployment_summary_topic_arn
      },
      {
        name  = "HEALTH_CHECK_TARGETS"
        value = jsonencode(local.health_check_targets)
      },
      {
        name  = "PORTAL_CATALOG_JSON"
        value = var.portal_catalog_json
      }
    ],
    var.github_token_secret_arn == "" && var.github_token != "" ? [{ name = "GITHUB_TOKEN", value = var.github_token }] : [],
    var.github_webhook_secret_arn == "" && var.github_webhook_secret != "" ? [{ name = "GITHUB_WEBHOOK_SECRET", value = var.github_webhook_secret }] : []
  )
  container_secrets = concat(
    var.github_token_secret_arn != "" ? [{ name = "GITHUB_TOKEN", valueFrom = var.github_token_secret_arn }] : [],
    var.github_webhook_secret_arn != "" ? [{ name = "GITHUB_WEBHOOK_SECRET", valueFrom = var.github_webhook_secret_arn }] : []
  )

  common_tags = merge(var.tags, {
    Environment = var.environment
    ManagedBy   = "terraform"
    Project     = var.project_name
  })
}
