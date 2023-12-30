# Substitute Go package name.
s|go.opentelemetry.io/proto/otlp|github.com/jaegertracing/jaeger/proto-gen/otel|g

# Substitute Proto package name.
s| opentelemetry.proto| jaeger|g

# Remove opentelemetry/proto prefix from imports.
s|import "opentelemetry/proto/|import "|g
