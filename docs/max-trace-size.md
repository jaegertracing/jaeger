# Max Trace Size Parameter

## Overview

Configurable limit on spans per trace to prevent Jaeger Query OOM issues when retrieving large traces. Exceeding traces are truncated with warning messages.

## Configuration

### Command Line
```bash
--query.max-trace-size=1000
```

### Config File
```yaml
query:
  max_trace_size: 1000
```

**Default**: `0` (no limit)

## How It Works

1. **Limit Check**: If `maxTraceSize > 0`, trace spans are limited to that number
2. **Truncation**: Only first `maxTraceSize` spans are kept
3. **Warning**: `jaeger.warning` tag added to first span: `"Trace truncated: only first 1000 spans loaded (total spans: 2500)"`

## Usage Examples

### Basic
```bash
jaeger-query --query.max-trace-size=1000
```

### Docker
```yaml
services:
  jaeger-query:
    image: jaegertracing/query:latest
    command: ["--query.max-trace-size=1000"]
```

## Best Practices

- **Small traces**: 100-1000 spans (no limit needed)
- **Medium traces**: 1000-10000 spans (consider 5000-10000 limit)  
- **Large traces**: 10000+ spans (recommend 10000-50000 limit)

## Troubleshooting

- **Incomplete traces**: Check if `maxTraceSize` is too low
- **High memory**: Verify configuration is applied
- **Debug**: Use `--log-level=debug` to see truncation events

## Related Issues

- [Jaeger Query OOM #1051](https://github.com/jaegertracing/jaeger/issues/1051)
- [Max trace size parameter #7495](https://github.com/jaegertracing/jaeger/issues/7495)


