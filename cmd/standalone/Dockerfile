FROM scratch

# Agent zipkin.thrift compact
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Agent config HTTP
EXPOSE 5778

# Collector HTTP
EXPOSE 14268

# Web HTTP
EXPOSE 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/
COPY ./cmd/standalone/sampling_strategies.json /go/src/config/

ENTRYPOINT ["/go/bin/standalone-linux", "--query.static-files=/go/src/jaeger-ui-build/build/", "--sampling.strategies-file=/go/src/config/sampling_strategies.json"]
