Throttling
==========

Purpose
-------

As of this writing, there is no way to prevent clients from flooding the Jaeger backend with debug spans. In fact, it is all too easy for a developer to forget to disable debug sampling after testing in a development environment. If the service is then deployed to a production environment, it could slow the Jaeger backend down to a crawl.

Throttling solves the issue of flooding by preventing clients from emitting multiple debug spans at once. Instead, the client maintains credits that it may use to emit debug spans. Otherwise, the debug span is simply emitted as a non-debug span.

Analysis
--------

A simple way to implement rate limiting is to use a token bucket. If that were our only concern, we would simply add a token bucket in the client. However, Jaeger can experience flooding in other ways. The main issue with a client-side mechanism comes up with short-lived programs like that have Jaeger integration. Each time the program runs, the client would reset its token bucket to full balance. This fact allows a caller to potentially generate multiple debug spans per second by invoking the program inside a loop. Consider this example of a bash command.

```
    $ while true; do jaeger-debug-program service-a; done
```

In order to effectively throttle debug traces, we need to maintain a token bucket outside of the process. In that way, we can maintain a limit over the total number of credits a service has, no matter how many times it runs and/or parallel instances it runs.

Implementation
--------------

Instead of a simple client-based rate limiter, we have decided to implement agent-based rate limiting. The way we do this is by creating a mapping between service names and token buckets.

### Throttling HTTP Service

During agent initialization, it initializes a throttling HTTP service. The throttling component creates an empty map that will later be used to map service names to token buckets. For simplicity, we call these *accounts*. When a client tracer initializes (using the throttling option), it instantiates a throttling client that periodically submits a request for debug credits. The client maintains its service name and a unique ID to identify the particular instance of the client (e.g. "service-a", 345).

When the agent receives a request for debug credits, it searches for an existing account for the service. If no account exists, the agent creates a new one (according to any service-specific configuration values passed to the throttler upon construction). Furthermore, the agent checks if it has already given this particular instance credits that it has not yet used. If it has, and those credits exceed the configured limit, the agent immediately rejects the request. Otherwise, the agent will grant the request and create an in-memory record of the count of the credits it granted the client. The client may then used these credits to emit debug spans.

### Emitting Debug Spans

If a client (using the throttling feature) tries to emit a debug span without any debug credits, one of two events may occur depending on the client configuration:

1.	The throttling client blocks on a request for credits to the agent. If the request succeeds, the client emits a debug span. Otherwise, the client request is rejected, and the client emits a regular span instead of a debug span.
2.	The throttling client immediately emits a regular span instead of a debug span. More credits will be requested on a periodic basis by the throttling client.

The choice is left to the user. We believe the first option makes sense with command-line tools mentioned above because they may not execute for more than a few seconds, thus never receive debug credits from the asynchronous approach. The second option is more appropriate for long-lived services.

If a debug span is successfully emitted, it finds its way to the agent as part of a batch of spans. When a client emits a batch of spans, it uses a process tag to emit the unique client ID along with the span data. Upon receiving the batch, the agent uses the client ID to update any balance associated with the client.
