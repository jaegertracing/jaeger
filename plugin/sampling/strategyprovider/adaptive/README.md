# Adaptive Sampling

Adaptive sampling works in Jaeger collector by observing the spans received from services and recalculating sampling probabilities for each service/endpoint combination to ensure that the volume of collected traces matches the desired target of traces per second. When a new service or endpoint is detected, it is initially sampled with "initial-sampling-probability" until enough data is collected to calculate the rate appropriate for the traffic going through the endpoint.

Adaptive sampling requires a storage backend to store the observed traffic data and computed probabilities. At the moment memory (for all-in-one deployment), cassandra, badger, elasticsearch and opensearch are supported as sampling storage backends.

References:
  * [Documentation](https://www.jaegertracing.io/docs/latest/sampling/#adaptive-sampling)
  * [Blog post](https://medium.com/jaegertracing/adaptive-sampling-in-jaeger-50f336f4334)
