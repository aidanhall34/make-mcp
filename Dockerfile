FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o ./bin/make-mcp ./cmd/mcp-server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o ./bin/make-mcp-validate ./cmd/validate

FROM golang:1.25 AS tests
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# OTEL_EXPORTER_OTLP_ENDPOINT: when set, test spans and metrics are sent to the
# LGTM instance at that address. Defaults to the dev stack endpoint. Pass an
# empty string to disable test telemetry export.
ARG OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
ENV OTEL_EXPORTER_OTLP_ENDPOINT=$OTEL_EXPORTER_OTLP_ENDPOINT
# -race uses ThreadSanitizer which requires a 48-bit VMA range. QEMU (used for
# cross-platform arm64 builds on amd64 hosts) only provides 47-bit, causing
# TSan to crash. Skip -race for arm64; it is already covered by the native
# amd64 container build and the unit-tests CI job.
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "arm64" ]; then \
        go test -cover -coverprofile=coverage.out ./... ; \
    else \
        go test -race -cover -coverprofile=coverage.out ./... ; \
    fi

FROM scratch AS app
WORKDIR /
# Force the tests stage: build fails here if coverage < 95%
COPY --from=tests /build/coverage.out /tmp/coverage.out
COPY --from=builder /build/bin/make-mcp /make-mcp
COPY --from=builder /build/bin/make-mcp-validate /make-mcp-validate
COPY make-mcp.yml /make-mcp.yml
ENTRYPOINT ["/make-mcp", "--config", "/make-mcp.yml"]
