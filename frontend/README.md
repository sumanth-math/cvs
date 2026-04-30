# Platform Resource Portal

React and TypeScript app for business users to request guarded S3 buckets through the platform API.

## Run Locally

Start the Go API on port `8080`:

```sh
go run ../cmd/platform-api
```

Install dependencies and start the frontend:

```sh
npm install
npm run dev
```

The Vite dev server proxies `/v1`, `/healthz`, and `/openapi.json` to `http://localhost:8080`.

To proxy local frontend requests to the deployed API:

```sh
VITE_API_PROXY_TARGET=http://platform-service-dev-1583960201.us-east-1.elb.amazonaws.com npm run dev
```

## Configuration

Set `VITE_API_BASE_URL` when a production build should call a deployed API directly:

```sh
VITE_API_BASE_URL=http://platform-service-dev-1583960201.us-east-1.elb.amazonaws.com npm run dev
```

For production builds:

```sh
npm run build
```
