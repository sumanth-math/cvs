# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/platform-api ./cmd/platform-api

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/platform-api /platform-api

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/platform-api"]
