# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.21.3 AS cert
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG TARGETARCH
EXPOSE 8080 8081 8082 8083
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY hotrod-linux-$TARGETARCH /go/bin/hotrod-linux
ENTRYPOINT ["/go/bin/hotrod-linux"]
CMD ["all"]

