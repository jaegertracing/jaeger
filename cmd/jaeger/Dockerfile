# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image=alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
ARG debug_image=ghcr.io/jaegertracing/base-image-with-debugger:0.1.0@sha256:dad2b5e8e26ea6d9092b8ecf4b582563376a8421fc44ed72f374b18ae840d5eb
# ------------- Begin PROD image -------------

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001

ENV JAEGER_LISTEN_HOST=0.0.0.0

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

ENV JAEGER_LISTEN_HOST=0.0.0.0

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
# Launch Jaeger under Delve debugger in headless mode but without pausing on start (--continue).
ENTRYPOINT ["/go/bin/dlv", "exec", "/cmd/jaeger/jaeger-linux", "--continue", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
