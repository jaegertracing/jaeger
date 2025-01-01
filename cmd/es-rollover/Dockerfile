# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

ARG base_image

FROM $base_image AS release
ARG TARGETARCH
ARG USER_UID=10001
COPY es-rollover-linux-$TARGETARCH /go/bin/es-rollover
ENTRYPOINT ["/go/bin/es-rollover"]
USER ${USER_UID}
