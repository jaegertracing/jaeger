# jaeger

This is experimental Jaeger V2 based on OpenTelemetry collector.
See https://github.com/jaegertracing/jaeger/issues/4843.

## Compatibility

### Service Name Sanitizer

In v1, there was a `serviceNameSanitizer` that sanitized the service names in span annotations using a source of truth alias to service cache. This functionality has been removed in v2. If your implementation relies on this sanitizer, you will need to find a different way to integrate this functionality, such as implementing a custom processor.