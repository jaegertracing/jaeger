# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image
ARG debug_image

# ------------- Begin PROD image -------------

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001

# Sampling config HTTP
EXPOSE 5778

# Sampling config gRPC
EXPOSE 5779

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

# Health Check gRPC
EXPOSE 13132

# Health Check HTTP
EXPOSE 13133

COPY jaeger-linux-$TARGETARCH /cmd/jaeger/jaeger-linux
COPY sampling-strategies.json /cmd/jaeger/sampling-strategies.json

VOLUME ["/tmp"]
ENTRYPOINT ["/cmd/jaeger/jaeger-linux"]
USER ${USER_UID}

# ------------- Begin DEBUG image -------------

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001

# Sampling config HTTP
EXPOSE 5778

# Sampling config gRPC
EXPOSE 5779

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

# Health Check gRPC
EXPOSE 13132

# Health Check HTTP
EXPOSE 13133

COPY jaeger-debug-linux-$TARGETARCH /cmd/jaeger/jaeger-linux
COPY sampling-strategies.json /cmd/jaeger/sampling-strategies.json

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/dlv", "exec", "/cmd/jaeger/jaeger-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
