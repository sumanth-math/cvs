output "alb_dns_name" {
  description = "DNS name for the platform API load balancer."
  value       = aws_lb.service.dns_name
}

output "api_url" {
  description = "HTTP URL for the platform API."
  value       = "http://${aws_lb.service.dns_name}"
}

output "ecr_repository_url" {
  description = "ECR repository URL for the service container."
  value       = aws_ecr_repository.service.repository_url
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.service.name
}

output "ecs_service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.service.name
}

output "vpc_id" {
  description = "VPC used by the platform API."
  value       = local.vpc_id
}

output "alb_subnet_ids" {
  description = "Subnets used by the Application Load Balancer."
  value       = local.alb_subnet_ids
}

output "service_subnet_ids" {
  description = "Subnets used by ECS Fargate tasks."
  value       = local.service_subnet_ids
}

output "task_role_arn" {
  description = "IAM role assumed by the platform API task."
  value       = aws_iam_role.task.arn
}

output "api_records_table_name" {
  description = "DynamoDB table that stores API output/audit records."
  value       = var.enable_api_records ? aws_dynamodb_table.api_records[0].name : null
}

output "api_records_table_arn" {
  description = "DynamoDB table ARN for API output/audit records."
  value       = var.enable_api_records ? aws_dynamodb_table.api_records[0].arn : null
}

output "observability_dashboard_name" {
  description = "CloudWatch dashboard for service observability."
  value       = var.enable_observability ? aws_cloudwatch_dashboard.service[0].dashboard_name : null
}

output "observability_dashboard_url" {
  description = "AWS console URL for the CloudWatch observability dashboard."
  value       = var.enable_observability ? "https://${var.aws_region}.console.aws.amazon.com/cloudwatch/home?region=${var.aws_region}#dashboards:name=${aws_cloudwatch_dashboard.service[0].dashboard_name}" : null
}

output "observability_sns_topic_arn" {
  description = "Managed SNS topic ARN for CloudWatch alarm notifications."
  value       = var.enable_observability && var.create_observability_sns_topic ? aws_sns_topic.observability[0].arn : null
}

output "github_actions_role_arn" {
  description = "Optional GitHub Actions OIDC role ARN."
  value       = local.create_github_role ? aws_iam_role.github_actions[0].arn : null
}
