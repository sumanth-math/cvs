terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.0"
    }
  }

  backend "s3" {
    bucket  = "sumanth-math-cvs-terraform-state"
    key     = "platform-service/terraform.tfstate"
    region  = "us-east-1"
    encrypt = true
  }
}
