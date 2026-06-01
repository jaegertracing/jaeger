# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image=alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001
COPY es-index-cleaner-linux-$TARGETARCH /go/bin/es-index-cleaner-linux
ENTRYPOINT ["/go/bin/es-index-cleaner-linux"]
USER ${USER_UID}
