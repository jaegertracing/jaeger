# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.23.2@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62 AS cert
RUN apk add --update --no-cache ca-certificates mailcap

FROM alpine:3.23.2@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=cert /etc/mime.types /etc/mime.types
