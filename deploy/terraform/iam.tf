data "aws_iam_policy_document" "ecs_tasks_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "task_execution" {
  name               = "${local.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "task" {
  name               = "${local.name}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

data "aws_iam_policy_document" "s3_provisioning" {
  statement {
    sid = "ProvisionPrefixedBuckets"
    actions = [
      "s3:CreateBucket",
      "s3:GetBucketLocation",
      "s3:PutBucketOwnershipControls",
      "s3:PutBucketPublicAccessBlock",
      "s3:PutBucketTagging",
      "s3:PutBucketVersioning",
      "s3:PutEncryptionConfiguration"
    ]
    resources = [
      "arn:${data.aws_partition.current.partition}:s3:::${var.managed_bucket_prefix}-*"
    ]
  }

  dynamic "statement" {
    for_each = length(var.allowed_kms_key_arns) > 0 ? [1] : []

    content {
      sid       = "ValidateAllowedKMSKeys"
      actions   = ["kms:DescribeKey"]
      resources = var.allowed_kms_key_arns
    }
  }
}

resource "aws_iam_policy" "s3_provisioning" {
  name        = "${local.name}-s3-provisioning"
  description = "Allows the platform API to provision guarded S3 buckets with the managed prefix."
  policy      = data.aws_iam_policy_document.s3_provisioning.json
}

resource "aws_iam_role_policy_attachment" "task_s3_provisioning" {
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.s3_provisioning.arn
}

data "aws_iam_policy_document" "api_records" {
  count = var.enable_api_records ? 1 : 0

  statement {
    sid       = "WriteAPIRecords"
    actions   = ["dynamodb:PutItem"]
    resources = [aws_dynamodb_table.api_records[0].arn]
  }
}

resource "aws_iam_policy" "api_records" {
  count = var.enable_api_records ? 1 : 0

  name        = "${local.name}-api-records"
  description = "Allows the platform API to record important API outputs in DynamoDB."
  policy      = data.aws_iam_policy_document.api_records[0].json
}

resource "aws_iam_role_policy_attachment" "task_api_records" {
  count = var.enable_api_records ? 1 : 0

  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.api_records[0].arn
}

data "aws_iam_policy_document" "deployment_summary_sns" {
  count = var.deployment_summary_topic_arn != "" ? 1 : 0

  statement {
    sid       = "PublishDeploymentSummaries"
    actions   = ["sns:Publish"]
    resources = [var.deployment_summary_topic_arn]
  }
}

resource "aws_iam_policy" "deployment_summary_sns" {
  count = var.deployment_summary_topic_arn != "" ? 1 : 0

  name        = "${local.name}-deployment-summary-sns"
  description = "Allows the platform API to publish GitHub deployment summaries."
  policy      = data.aws_iam_policy_document.deployment_summary_sns[0].json
}

resource "aws_iam_role_policy_attachment" "deployment_summary_sns" {
  count = var.deployment_summary_topic_arn != "" ? 1 : 0

  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.deployment_summary_sns[0].arn
}

data "aws_iam_policy_document" "github_webhook_secrets" {
  count = length(local.github_secret_arns) > 0 ? 1 : 0

  statement {
    sid = "ReadGitHubWebhookSecrets"
    actions = [
      "secretsmanager:GetSecretValue",
      "ssm:GetParameters"
    ]
    resources = local.github_secret_arns
  }

  dynamic "statement" {
    for_each = length(var.github_secret_kms_key_arns) > 0 ? [1] : []

    content {
      sid       = "DecryptGitHubWebhookSecrets"
      actions   = ["kms:Decrypt"]
      resources = var.github_secret_kms_key_arns
    }
  }
}

resource "aws_iam_policy" "github_webhook_secrets" {
  count = length(local.github_secret_arns) > 0 ? 1 : 0

  name        = "${local.name}-github-webhook-secrets"
  description = "Allows ECS to inject GitHub webhook token and secret values."
  policy      = data.aws_iam_policy_document.github_webhook_secrets[0].json
}

resource "aws_iam_role_policy_attachment" "github_webhook_secrets" {
  count = length(local.github_secret_arns) > 0 ? 1 : 0

  role       = aws_iam_role.task_execution.name
  policy_arn = aws_iam_policy.github_webhook_secrets[0].arn
}
