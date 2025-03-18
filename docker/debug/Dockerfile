# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.24.1-alpine@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386 AS build
ARG TARGETARCH
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git build-base mailcap

COPY go.mod go.sum /go/src/debug-delve/ 

# TODO: Remove s390x once go-delve adds support for it (https://github.com/go-delve/delve/issues/2883)
# TODO: Remove ppc64le once support is released (https://github.com/go-delve/delve/issues/1564) - not yet as of delve@v1.22.1
RUN if [[ "$TARGETARCH" == "s390x" ||  "$TARGETARCH" == "ppc64le" ]] ; then \
	touch /go/bin/dlv; \
    else \
        cd /go/src/debug-delve && go mod download && go build -o /go/bin/dlv github.com/go-delve/delve/cmd/dlv; \
    fi 

FROM golang:1.24.1-alpine@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386
COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/mime.types /etc/mime.types
