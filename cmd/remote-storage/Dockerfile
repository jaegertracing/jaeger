# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image=alpine@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
ARG debug_image=ghcr.io/jaegertracing/base-image-with-debugger:0.1.0@sha256:dad2b5e8e26ea6d9092b8ecf4b582563376a8421fc44ed72f374b18ae840d5eb
ARG SVC=remote-storage

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001
COPY remote-storage-linux-$TARGETARCH /go/bin/remote-storage-linux
EXPOSE 16686/tcp
ENTRYPOINT ["/go/bin/remote-storage-linux"]
USER ${USER_UID}

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY remote-storage-debug-linux-$TARGETARCH /go/bin/remote-storage-linux
EXPOSE 12345/tcp 16686/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/remote-storage-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}
