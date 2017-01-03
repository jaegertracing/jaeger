# `jaeger-agent` sidecar service

Agent is meant to run on each host that runs the services instrumented with Jaeger. Jaeger client libraries send tracing spans to `jaeger-agent`. The agent forwards the spans to `jaeger-collector` services for storing in the DB.
