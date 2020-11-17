ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64

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

# Collector gRPC
EXPOSE 14250

# Web HTTP
EXPOSE 16686

COPY all-in-one-linux-$TARGETARCH /go/bin/all-in-one-linux
COPY sampling_strategies.json /etc/jaeger/

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/all-in-one-linux"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64

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

# Collector gRPC
EXPOSE 14250

# Web HTTP
EXPOSE 16686

# Delve
EXPOSE 12345

COPY all-in-one-debug-linux-$TARGETARCH /go/bin/all-in-one-linux
COPY sampling_strategies.json /etc/jaeger/

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/all-in-one-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]
