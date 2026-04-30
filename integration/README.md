# Integration Tests

This folder contains HTTP-level integration tests for the deployed platform API.
They exercise live service behavior through `PLATFORM_API_BASE_URL` instead of
using in-memory handlers.

The tests are intentionally read-only or validation-only. They check health,
OpenAPI, catalog responses, JSON error handling, and S3 bucket request
validation without creating AWS resources.

Run them against a deployed API:

```sh
PLATFORM_API_BASE_URL=http://platform-service-dev-example.us-east-1.elb.amazonaws.com go test -v ./integration/...
```

If `PLATFORM_API_BASE_URL` is not set, the tests skip themselves so the regular
unit test workflow can still run locally.
