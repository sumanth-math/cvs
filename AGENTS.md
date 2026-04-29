# AI Agent Configuration

## Project Context

This repository contains a Go platform self-service API that runs on AWS ECS Fargate and provisions guarded S3 buckets for development teams. Infrastructure lives in Terraform under `deploy/terraform`, and GitHub Actions deploys the workload.

## Agent Guidelines

- Prefer small, focused changes over broad rewrites.
- Read existing files before editing, and preserve user changes.
- Use established project patterns when they exist.
- Avoid adding new dependencies unless they are clearly needed.
- Keep generated files, build artifacts, and secrets out of version control.

## Development Commands

- Install dependencies: `go mod download`
- Run tests: `go test ./...`
- Start development server: `go run ./cmd/platform-api`
- Build container: `docker build -t platform-service:local .`
- Format Terraform: `terraform fmt -recursive deploy/terraform`
- Validate Terraform: `terraform -chdir=deploy/terraform validate`

## Code Style

- Keep code readable and idiomatic for the chosen language or framework.
- Add comments only when they clarify non-obvious behavior.
- Prefer explicit names for files, functions, and configuration.
- Keep configuration close to the code it affects unless the tool expects a root-level file.
- Keep platform guardrails in the API and IAM policy aligned; do not allow self-service resources outside the managed prefix without an explicit design change.

## Testing Expectations

- Add or update tests for behavioral changes.
- Run the narrowest relevant checks first, then broader checks when practical.
- If tests cannot be run, note why and describe the remaining risk.

## Safety Boundaries

- Do not commit credentials, tokens, private keys, or local environment files.
- Do not run destructive commands unless explicitly requested.
- Do not overwrite unrelated user work.
