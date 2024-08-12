# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM scratch
ARG TARGETARCH
ARG USER_UID=10001

COPY anonymizer-linux-$TARGETARCH /go/bin/anonymizer-linux
ENTRYPOINT ["/go/bin/anonymizer-linux"]
USER ${USER_UID}
