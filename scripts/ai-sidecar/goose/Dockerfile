# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This image runs a released Goose binary as the ACP agent of the stack in
# docker-compose.yml. Goose publishes no versioned container image, so the
# binary is taken from its GitHub release for the build platform.
FROM alpine:3.21
ARG GOOSE_VERSION=v1.57.0
ARG TARGETARCH
RUN apk add --no-cache ca-certificates curl \
  && case "$TARGETARCH" in amd64) arch=x86_64 ;; arm64) arch=aarch64 ;; *) echo "unsupported arch $TARGETARCH" && exit 1 ;; esac \
  && curl -fsSL "https://github.com/aaif-goose/goose/releases/download/${GOOSE_VERSION}/goose-${arch}-unknown-linux-musl.tar.gz" \
     | tar -xz -C /usr/local/bin ./goose \
  && apk del curl
ENV HOME=/tmp
EXPOSE 16688
ENTRYPOINT ["goose", "serve", "--host", "0.0.0.0", "--port", "16688"]
