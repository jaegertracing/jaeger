#Architecture
##Overview
Jaeger's architecture is an OpenTracing implementation, and adheres to the data model described in the Opentracing standard. 
Reading the [specification](https://github.com/opentracing/specification/blob/master/specification.md) will help you understand this section better.

##Terminology
###Span
A **Span** represents a logical unit of work in the system that has an operation name, the start time of the operation, and the duration. Spans may be nested and ordered to model causal relationships. An RPC call is an example of a span.  

![Traces And Spans](images/spans-traces.png)
*Traces and Spans*

###Trace
A **Trace** is a data/execution path through the system, and can be thought of as a directed acyclic graph of spans


##Components
![Architecture](images/architecture.png)
*The current Jaeger architecture*

This section details the constituents of Jaeger, and how they relate to each other. It is arranged by the order in which spans from your application interact with them. 

###Jaeger client
Jaeger clients are language specific implementations of the OpenTracing API. The contain a Tracer, which is an interface for Span creation, and context propagation. 
Some common frameworks (e.g. dropwizard) are instrumented out of the box, making it easy to get started. 

An instrumented library creates spans when receiving new requests, and attaches context information (trace id, and span id) to outgoing requests. Note that only ids are propagated with requests, and any other information like operation name, logs, etc is not propagated, and is simply collected at the service by the Jaeger client.

The instrumentation has very little overhead, and is designed to be always enabled in production.

Note that while all traces are instrumented, only few are sampled. Sampling a trace marks the trace for further processing and storage. 
By default, Jaeger client samples 0.0001% of traces, and has the ability to retrieve to retrieve sampler parameters from the agent. 

![Context propagation explained](images/context-prop.png)
*Context propagation*

###Agent
A network daemon that lists for spans sent over UDP, which it batches and sends to the collector. It is designed to be deployed to all hosts as an infrastructure component.  The agent abstracts the routing and discovery of the collectors away from the client. 

###Collector
The collector receives traces from Jaeger agents, runs them through a processing pipeline.. Currently our pipeline validates traces, indexes them, performs any transformations, and finally stores them. 
Our storage is a pluggable component, which currently supports Cassandra. 

###Query
Query is a service that retrieves traces from storage, and hosts a UI to display them. 

