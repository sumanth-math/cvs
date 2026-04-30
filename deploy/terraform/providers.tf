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

  common_tags = merge(var.tags, {
    Environment = var.environment
    ManagedBy   = "terraform"
    Project     = var.project_name
  })
}
