# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001

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
ENV SAMPLING_STRATEGIES_FILE=/etc/jaeger/sampling_strategies.json

COPY all-in-one-linux-$TARGETARCH /go/bin/all-in-one-linux
COPY sampling_strategies.json /etc/jaeger/

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/all-in-one-linux"]
USER ${USER_UID}

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001

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
ENV SAMPLING_STRATEGIES_FILE=/etc/jaeger/sampling_strategies.json

COPY all-in-one-debug-linux-$TARGETARCH /go/bin/all-in-one-linux
COPY sampling_strategies.json /etc/jaeger/

VOLUME ["/tmp"]
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/all-in-one-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
