# Client Libraries

## Official libraries

| Language | Library                                                      | 
| ---------|--------------------------------------------------------------|
| go       | [jaeger-client-go](https://github.com/uber/jaeger-client-go)        |
| java     | [jaeger-client-java](https://github.com/uber/jaeger-client-java)    |
| node     | [jaeger-client-node](https://github.com/uber/jaeger-client-node)    |
| python   | [jaeger-client-python](https://github.com/uber/jaeger-client-python)|

For a deep dive into how to instrument a Go service, look at [Tracing HTTP request latency](https://medium.com/@YuriShkuro/tracing-http-request-latency-in-go-with-opentracing-7cc1282a100a).

### Initializing Jaeger Tracer

The initialization syntax is slightly different in each langauges, please refer to the README's in the respective repositories.
The general pattern is to not create the Tracer explicitly, but use a Configuration class to do that.  Configuration allows
simpler parameterization of the Tracer, such as changing the default sampler or the location of Jaeger agent.

### EMSGSIZE and UDP buffer limits

By default Jaeger libraries use a UDP sender to report finished spans to `jaeger-agent` sidecar.
The default max packet size is 65,000 bytes, which can be transmitted without segmentation when
connecting to the agent via loopback interface. However, some OSs (in particular, MacOS), limit
the max buffer size for UDP packets, as raised in [this GitHub issue](https://github.com/uber/jaeger-client-node/issues/124).
If you run into issue with `EMSGSIZE` errors, consider raising the limits in your kernel (see the issue for examples).
You can also configure the client libraries to use a smaller max packet size, but that may cause
issues if you have large spans, e.g. if you log big chunks of data. Spans that exceed max packet size
are dropped by the clients (with metrics emitted to indicate that). Another alternative is
to use non-UDP transports, such as [HttpSender in Java][HttpSender] (not currently available for all langauges).

## OpenTracing Contributions

See the OpenTracing contributions repository on [Github](https://github.com/opentracing-contrib) for more libraries. 

[HttpSender]: /https://github.com/uber/jaeger-client-java/blob/master/jaeger-core/src/main/java/com/uber/jaeger/senders/HttpSender.java
