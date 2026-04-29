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
