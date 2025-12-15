# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.23.0@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375 AS cert
RUN apk add --update --no-cache ca-certificates mailcap

FROM alpine:3.23.0@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375
# When using a newer base image, you generally don't need to pin busybox versions,
# as the base image ensures the most current/stable versions are installed by default.
RUN apk add --no-cache \
    busybox \
    busybox-binsh \
    ssl_client
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=cert /etc/mime.types /etc/mime.types
