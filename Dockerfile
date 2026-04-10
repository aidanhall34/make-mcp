FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o ./bin/mcp-server ./cmd/mcp-server

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
RUN go test -race -cover -coverprofile=coverage.out ./...

FROM alpine:3.21 AS app
# make is required at runtime: the server invokes it to run recipe targets.
RUN apk add --no-cache make
WORKDIR /opt/make-mcp
# Force the tests stage: build fails here if coverage < 95%
COPY --from=tests /build/coverage.out /tmp/coverage.out
COPY --from=builder /build/bin/mcp-server ./mcp-server
COPY make-mcp.yml ./make-mcp.yml
ENTRYPOINT ["/opt/make-mcp/mcp-server", "--config", "/opt/make-mcp/make-mcp.yml"]
