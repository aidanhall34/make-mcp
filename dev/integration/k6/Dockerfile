# Builds a custom k6 binary with the xk6-mcp extension bundled in.
# The resulting image is used by docker-compose.integration.yml.
FROM grafana/xk6 AS builder
# GONOSUMDB bypasses the checksum database for pre-release modules.
# GOFLAGS ensures the go.sum is updated automatically (required in a fresh module).
ENV GONOSUMDB=*
ENV GOFLAGS=-mod=mod
RUN xk6 build \
    --with github.com/dgzlopes/xk6-mcp@v0.0.3 \
    --output /xk6/k6

FROM alpine:3.21
COPY --from=builder /xk6/k6 /usr/local/bin/k6
ENTRYPOINT ["k6"]
