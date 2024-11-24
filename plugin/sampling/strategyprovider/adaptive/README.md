# Adaptive Sampling

Adaptive sampling works in Jaeger collector by observing the spans received from services and recalculating sampling probabilities for each service/endpoint combination to ensure that the volume of collected traces matches the desired target of traces per second. When a new service or endpoint is detected, it is initially sampled with "initial-sampling-probability" until enough data is collected to calculate the rate appropriate for the traffic going through the endpoint.

Adaptive sampling requires a storage backend to store the observed traffic data and computed probabilities. At the moment memory (for all-in-one deployment), cassandra, badger, elasticsearch and opensearch are supported as sampling storage backends.

Note: adaptive sampling in Jaeger backend *does not actually do the sampling*. The sampling is performed by OTEL SDKs, and sampling decisions are propagated through trace context. The job of Adaptive Sampling is to dynamically calculate sampling probabilities and expose them as sampling strategies via Jaeger's Remote Sampling protocol.

References:
  * [Documentation](https://www.jaegertracing.io/docs/latest/sampling/#adaptive-sampling)
  * [Blog post](https://medium.com/jaegertracing/adaptive-sampling-in-jaeger-50f336f4334)

## Implementation details

There are three main components of the Adaptive Sampling: Aggregator, Post-aggregator (could use a better name), and Provider.

### Aggregator

*Aggregator* is a component that runs in the ingestion pipeline (e.g. as a trace processor in OTEL Collector). It looks at all spans passing through that instance of the collector and looks for root spans. Each root span indicates a new trace being generated, so the aggregation aggregates the count of those traces (grouped by service name and span name) and periodically flushes those aggregates (called "throughput") to storage.

### Post-aggregator

*Post-aggregator* is the main logic responsible for _adaptive_ part of this sampling strategy implementation. Its main job is to load all throughput from storage (because multiple instances of collector could've written different aggregates), aggregate it into a final output, and compute the desired sampling probabilities, which are also written into storage.

In a typical production usage Jaeger deployment consists of many collectors. Each collector runs an independent aggregator, because they do not require coordination as long as there is a shared storage. Each collector also runs post-aggregator, however only one of those should be combining the output of all aggregators and producing the final sampling probabilities. This is achieved by using a simple leader-follower election with the help of the storage backend. The leader post-aggregator does the main job of the computation, while the follower-aggregators are only loading the throughput from storage and aggregate it in memory, so that each of them is ready to assume the role of the leader if needed, but they do not compute the probabilities or write them back into storage.

### Provider

*Provider* is responsible for providing the sampling strategy to the SDKs when they poll the `/sampling` endpoint. It periodically reads the computed sampling probabilities from storage and translates them into sampling strategy output expected by the Jaeger Remote Sampling protocol.
