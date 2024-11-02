# Jaeger Agent

`jaeger-agent` is a daemon program that runs on every host and receives
tracing information submitted by applications via Jaeger client 
libraries.

## Structure

* Agent
    * processor as ThriftProcessor
        * server as TBufferedServer
            * Thrift UDP Transport
        * reporter as CollectorReporter
    * sampling server
        * sampling manager as sampling.CollectorProxy

### UDP Server

Listens on UDP transport, reads data as `[]byte` and forwards to
`processor` over channel. Processor has N workers that read from
the channel, convert to thrift-generated object model, and pass on
to the Reporter. `CollectorReporter` submits the spans to remote
`collector` service.

### Sampling Server

An HTTP server handling request in the form

    http://localhost:port/sampling?service=xxxx`

Delegates to `sampling.Manager` to get the sampling strategy.
`sampling.CollectorProxy` implements `sampling.Manager` by querying
remote `collector` service. Then the server converts
thrift response from sampling manager into JSON and responds to clients.

