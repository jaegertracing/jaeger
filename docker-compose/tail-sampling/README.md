# Tail-Based Sampling Processor

This `docker compose` environment provides a sample configuration of a Jaeger collector utilizing the
[Tail-Based Sampling Processor in OpenTelemtry](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md).

## Description of Setup

The `docker-compose.yml` contains three services and their functions are outlined as follows:

1. `jaeger` - This is the Jaeger V2 collector that samples traces using the `tail_sampling` processor.
The configuration for this service is in [jaeger-v2-config.yml](./jaeger-v2-config.yml).
The `tail_sampling` processor has one policy that only captures traces from the services `tracegen-02` and `tracegen-04`.
For a full list of policies that can be added to the `tail_sampling` processor, check out [this README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md).
2. `otel_collector` - This is an OpenTelemtry collector with a `loadbalancing` exporter that routes requests to `jaeger`.
The configuration for this service is in [otel-collector-config-connector.yml](./otel-collector-config-connector.yml).
The purpose of this collector is to collect spans from different services and forward all spans with the same `traceID`
to the same downstream collector instance (`jaeger` in this case), so that sampling decisions for a given trace can be
made in the same collector instance.
3. `tracegen` - This is a service that generates traces for 5 different services and sends them to `otel_collector`
(which will in turn send them to `jaeger`).

Note that in this minimal setup, a `loadbalancer` collector is not necessary since we are only running a
single instance of the `jaeger` collector. In a real-world distributed system running multiple instances
of the `jaeger` collector, a load balancer is necessary to avoid spans from the same trace being routed
to different collector instances.

## Running the Example

The example can be run using the following command:

```bash
make dev
```

To see the tail-based sampling processor in action, go to the Jaeger UI at <http://localhost:16686/>. 
You will see that only traces for the services outlined in the policy in [jaeger-v2-config.yml](./jaeger-v2-config.yml) 
are sampled.
