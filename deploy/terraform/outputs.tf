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

output "task_role_arn" {
  description = "IAM role assumed by the platform API task."
  value       = aws_iam_role.task.arn
}

output "github_actions_role_arn" {
  description = "Optional GitHub Actions OIDC role ARN."
  value       = local.create_github_role ? aws_iam_role.github_actions[0].arn : null
}
