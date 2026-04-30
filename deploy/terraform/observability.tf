locals {
  observability_alarm_topic_arns = var.enable_observability ? distinct(concat(
    var.alarm_notification_topic_arns,
    var.create_observability_sns_topic ? [aws_sns_topic.observability[0].arn] : []
  )) : []
}

resource "aws_sns_topic" "observability" {
  count = var.enable_observability && var.create_observability_sns_topic ? 1 : 0

  name = "${local.name}-observability-alerts"
}

resource "aws_sns_topic_subscription" "observability_email" {
  for_each = var.enable_observability && var.create_observability_sns_topic ? toset(var.alarm_email_endpoints) : []

  topic_arn = aws_sns_topic.observability[0].arn
  protocol  = "email"
  endpoint  = each.value
}

resource "aws_cloudwatch_metric_alarm" "ecs_cpu_high" {
  count = var.enable_observability ? 1 : 0

  alarm_name          = "${local.name}-ecs-cpu-high"
  alarm_description   = "ECS service average CPU utilization is above ${var.alarm_cpu_threshold}%."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 5
  datapoints_to_alarm = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = var.alarm_cpu_threshold
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.observability_alarm_topic_arns
  ok_actions          = local.observability_alarm_topic_arns

  dimensions = {
    ClusterName = aws_ecs_cluster.service.name
    ServiceName = aws_ecs_service.service.name
  }
}

resource "aws_cloudwatch_metric_alarm" "ecs_memory_high" {
  count = var.enable_observability ? 1 : 0

  alarm_name          = "${local.name}-ecs-memory-high"
  alarm_description   = "ECS service average memory utilization is above ${var.alarm_memory_threshold}%."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 5
  datapoints_to_alarm = 3
  metric_name         = "MemoryUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = var.alarm_memory_threshold
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.observability_alarm_topic_arns
  ok_actions          = local.observability_alarm_topic_arns

  dimensions = {
    ClusterName = aws_ecs_cluster.service.name
    ServiceName = aws_ecs_service.service.name
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_target_5xx" {
  count = var.enable_observability ? 1 : 0

  alarm_name          = "${local.name}-alb-target-5xx"
  alarm_description   = "ALB target 5xx responses are above ${var.alarm_5xx_threshold} per minute."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 5
  datapoints_to_alarm = 3
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Sum"
  threshold           = var.alarm_5xx_threshold
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.observability_alarm_topic_arns
  ok_actions          = local.observability_alarm_topic_arns

  dimensions = {
    LoadBalancer = aws_lb.service.arn_suffix
    TargetGroup  = aws_lb_target_group.service.arn_suffix
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_unhealthy_hosts" {
  count = var.enable_observability ? 1 : 0

  alarm_name          = "${local.name}-alb-unhealthy-hosts"
  alarm_description   = "ALB reports unhealthy ECS targets."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 3
  datapoints_to_alarm = 2
  metric_name         = "UnHealthyHostCount"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Average"
  threshold           = var.alarm_unhealthy_host_threshold
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.observability_alarm_topic_arns
  ok_actions          = local.observability_alarm_topic_arns

  dimensions = {
    LoadBalancer = aws_lb.service.arn_suffix
    TargetGroup  = aws_lb_target_group.service.arn_suffix
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_target_response_time" {
  count = var.enable_observability ? 1 : 0

  alarm_name          = "${local.name}-alb-target-latency-high"
  alarm_description   = "ALB target response time is above ${var.alarm_target_response_time_seconds} seconds."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 5
  datapoints_to_alarm = 3
  metric_name         = "TargetResponseTime"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Average"
  threshold           = var.alarm_target_response_time_seconds
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.observability_alarm_topic_arns
  ok_actions          = local.observability_alarm_topic_arns

  dimensions = {
    LoadBalancer = aws_lb.service.arn_suffix
    TargetGroup  = aws_lb_target_group.service.arn_suffix
  }
}

resource "aws_cloudwatch_dashboard" "service" {
  count = var.enable_observability ? 1 : 0

  dashboard_name = "${local.name}-observability"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "text"
        x      = 0
        y      = 0
        width  = 24
        height = 3
        properties = {
          markdown = "# ${local.name} observability\n\nAPI: http://${aws_lb.service.dns_name}\n\nLog group: ${aws_cloudwatch_log_group.service.name}"
        }
      },
      {
        type   = "metric"
        x      = 0
        y      = 3
        width  = 12
        height = 6
        properties = {
          title   = "ECS CPU and memory"
          region  = var.aws_region
          view    = "timeSeries"
          stacked = false
          period  = 60
          metrics = [
            ["AWS/ECS", "CPUUtilization", "ClusterName", aws_ecs_cluster.service.name, "ServiceName", aws_ecs_service.service.name, { label = "CPU %" }],
            [".", "MemoryUtilization", ".", ".", ".", ".", { label = "Memory %" }]
          ]
          yAxis = {
            left = {
              min = 0
              max = 100
            }
          }
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 3
        width  = 12
        height = 6
        properties = {
          title   = "ALB traffic and latency"
          region  = var.aws_region
          view    = "timeSeries"
          stacked = false
          period  = 60
          metrics = [
            ["AWS/ApplicationELB", "RequestCount", "LoadBalancer", aws_lb.service.arn_suffix, { stat = "Sum", label = "Requests" }],
            [".", "TargetResponseTime", ".", ".", { stat = "Average", label = "Avg target response time" }]
          ]
        }
      },
      {
        type   = "metric"
        x      = 0
        y      = 9
        width  = 12
        height = 6
        properties = {
          title   = "ALB target errors"
          region  = var.aws_region
          view    = "timeSeries"
          stacked = false
          period  = 60
          metrics = [
            ["AWS/ApplicationELB", "HTTPCode_Target_5XX_Count", "LoadBalancer", aws_lb.service.arn_suffix, "TargetGroup", aws_lb_target_group.service.arn_suffix, { stat = "Sum", label = "Target 5xx" }],
            [".", "HTTPCode_Target_4XX_Count", ".", ".", ".", ".", { stat = "Sum", label = "Target 4xx" }]
          ]
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 9
        width  = 12
        height = 6
        properties = {
          title   = "ALB target health"
          region  = var.aws_region
          view    = "timeSeries"
          stacked = false
          period  = 60
          metrics = [
            ["AWS/ApplicationELB", "HealthyHostCount", "LoadBalancer", aws_lb.service.arn_suffix, "TargetGroup", aws_lb_target_group.service.arn_suffix, { stat = "Average", label = "Healthy hosts" }],
            [".", "UnHealthyHostCount", ".", ".", ".", ".", { stat = "Average", label = "Unhealthy hosts" }]
          ]
        }
      },
      {
        type   = "log"
        x      = 0
        y      = 15
        width  = 24
        height = 8
        properties = {
          title  = "Recent service logs"
          region = var.aws_region
          view   = "table"
          query  = "SOURCE '${aws_cloudwatch_log_group.service.name}' | fields @timestamp, @message | sort @timestamp desc | limit 25"
        }
      }
    ]
  })
}
