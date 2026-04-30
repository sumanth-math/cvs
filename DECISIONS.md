# Design Decisions

## Centralized Self-Service API

The main design decision was to put cloud provisioning behind a centralized Go API running on ECS Fargate, instead of giving each development team direct AWS permissions or asking every team to run Terraform modules themselves. This keeps the developer workflow simple while letting the platform team enforce naming, tagging, encryption, public access blocking, and IAM boundaries in one place. ECS Fargate was chosen because the service is long-running, HTTP-native, easy to place behind an ALB, and fits the existing Terraform and GitHub Actions deployment model.

Alternatives considered were a Lambda-based API, direct GitHub Actions workflows for each request, and reusable Terraform modules consumed by application teams. Lambda would reduce server management but makes a growing workflow handler and HTTP API less straightforward to operate as one service. Direct GitHub Actions or team-owned Terraform modules would be flexible, but they would spread provisioning logic and permissions across repositories. The centralized service trades a small amount of platform ownership for stronger guardrails, auditability through DynamoDB, and a cleaner self-service experience for developers.
