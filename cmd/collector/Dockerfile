ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001
COPY collector-linux-$TARGETARCH /go/bin/collector-linux

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

ENTRYPOINT ["/go/bin/collector-linux"]
USER ${USER_UID}

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY collector-debug-linux-$TARGETARCH /go/bin/collector-linux

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

# Delve
EXPOSE 12345

ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/collector-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
