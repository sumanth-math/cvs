resource "terraform_data" "network_validation" {
  input = local.create_network

  lifecycle {
    precondition {
      condition     = local.create_network || (trimspace(var.vpc_id) != "" && length(var.alb_subnet_ids) >= 2 && length(var.private_subnet_ids) >= 1)
      error_message = "Set vpc_id, at least two alb_subnet_ids, and at least one private_subnet_id, or leave all three empty so Terraform creates a demo network."
    }
  }
}

resource "aws_vpc" "managed" {
  count = local.create_network ? 1 : 0

  cidr_block           = "10.80.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "${local.name}-vpc"
  }
}

resource "aws_internet_gateway" "managed" {
  count = local.create_network ? 1 : 0

  vpc_id = aws_vpc.managed[0].id

  tags = {
    Name = "${local.name}-igw"
  }
}

resource "aws_subnet" "public" {
  count = local.create_network ? 2 : 0

  vpc_id                  = aws_vpc.managed[0].id
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  cidr_block              = cidrsubnet(aws_vpc.managed[0].cidr_block, 4, count.index)
  map_public_ip_on_launch = true

  tags = {
    Name = "${local.name}-public-${count.index + 1}"
  }
}

resource "aws_route_table" "public" {
  count = local.create_network ? 1 : 0

  vpc_id = aws_vpc.managed[0].id

  tags = {
    Name = "${local.name}-public"
  }
}

resource "aws_route" "public_internet" {
  count = local.create_network ? 1 : 0

  route_table_id         = aws_route_table.public[0].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.managed[0].id
}

resource "aws_route_table_association" "public" {
  count = local.create_network ? length(aws_subnet.public) : 0

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public[0].id
}
