ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001

# Agent zipkin.thrift compact
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Agent config HTTP
EXPOSE 5778

# Collector OTLP gRPC
EXPOSE 4317

# Collector OTLP HTTP
EXPOSE 4318

# Collector HTTP
EXPOSE 14268

# Collector gRPC
EXPOSE 14250

# Collector Zipkin
EXPOSE 9411

# Web HTTP
EXPOSE 16686

# Default configuration file for setting sampling strategies
# ENV SAMPLING_STRATEGIES_FILE=/etc/jaeger/sampling_strategies.json
# COPY sampling_strategies.json /etc/jaeger/

COPY jaeger-linux-$TARGETARCH /go/bin/jaeger-linux

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/jaeger-linux"]
USER ${USER_UID}

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001

# Agent zipkin.thrift compact
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Agent config HTTP
EXPOSE 5778

# Collector OTLP gRPC
EXPOSE 4317

# Collector OTLP HTTP
EXPOSE 4318

# Collector HTTP
EXPOSE 14268

# Collector gRPC
EXPOSE 14250

# Collector Zipkin
EXPOSE 9411

# Web HTTP
EXPOSE 16686

# Delve
EXPOSE 12345

# Default configuration file for setting sampling strategies
# ENV SAMPLING_STRATEGIES_FILE=/etc/jaeger/sampling_strategies.json
# COPY sampling_strategies.json /etc/jaeger/

COPY jaeger-debug-linux-$TARGETARCH /go/bin/jaeger-linux

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/jaeger-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
