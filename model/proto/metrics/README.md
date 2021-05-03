# Metrics Query Service

Defines the MetricsQueryService's set of APIs along with required data models.

## Overview

Contained in this directory are a set of shared Protobuf data model definitions from
https://github.com/open-telemetry/opentelemetry-proto; namely:

- otelmetric.proto: OpenTelemetry's Metric data model.
- otelspankind.proto: OpenTelemetry's SpanKind data model.

The reasons for adopting OpenTelemetry's metrics data model are:

- There is a basic requirement to "normalize" and un/marshal metrics queried from a backing store
  for clients, that is agnostic to the backing store implementation.
- The data being queried is sourced from an OpenTelemetry Collector.

Importing data models directly from the open-telemetry/opentelemetry-proto github repo (via a submodule)
was considered and explored; however, without custom marshaling enabled, which is required for sending
imported message types over the wire, errors such as the following result:

    `panic: invalid Go type v1.Metric for field jaeger.api_v2.GetMetricsResponse.Metrics`

Enabling gogoproto's custom Marshal and Unmarshal methods to address the above issue result
in compilation errors from the generated code as opentelemetry-proto's Metric type does not have
gogoproto.marshaler_all, gogoproto.unmarshaler_all, etc. enabled.

Moreover, if direct imports of other repositories were possible, it would mean importing and generating code for
transitive dependencies not required by Jaeger leading to longer build times, and potentially larger container
image sizes.

Given the aforementioned limitations, selectively copying necessary messages and enums allow for:

- Marshaling and unmarshaling of externally defined custom data models such as those from OpenTelemetry.
- Using Gogoproto's custom un/marshalers takes advantage of [reportedly faster marshaling and
  unmarshaling](https://github.com/cockroachdb/gogoproto/blob/master/extensions.md).
- Avoiding unwanted dependencies leading to simpler proto definitions,
  faster build times and smaller image sizes.

The key trade-offs are:

- Synchronizing with the original source proto definition.
  - It is anticipated that the maintenance effort to synchronize data models will be minimal considering
    there is no direct dependency between Jaeger and OpenTelemetry in the context of querying metrics,
    with exception to `SpanKind`, and the existing data model more than satisfies existing metrics querying requirements.

    The OpenTelemetry metrics data model primarily serves as a carrier of metrics data, rather than a protocol
    of communication between Jaeger and OpenTelemetry components.
