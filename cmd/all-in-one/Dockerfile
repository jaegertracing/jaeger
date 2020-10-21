ARG TARGET=release

FROM golang:1.15-alpine as build
ENV GOPATH /go

RUN apk add --update --no-cache ca-certificates make git && \
    go get github.com/go-delve/delve/cmd/dlv && \
    cd /go/src/github.com/go-delve/delve && \
    make install

FROM golang:1.15-alpine AS base-image-debug

COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

FROM alpine:3.12 AS base-image-release

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

FROM base-image-${TARGET} as common

# Agent zipkin.thrift compact
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Agent config HTTP
EXPOSE 5778

# Collector HTTP
EXPOSE 14268

# Collector gRPC
EXPOSE 14250

# Web HTTP
EXPOSE 16686

COPY sampling_strategies.json /etc/jaeger/

VOLUME ["/tmp"]

FROM common AS release

ARG TARGETARCH
COPY all-in-one-linux-${TARGETARCH} /go/bin/all-in-one-linux

ENTRYPOINT ["/go/bin/all-in-one-linux"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]

FROM common AS debug

ARG TARGETARCH
COPY all-in-one-debug-linux-${TARGETARCH} /go/bin/all-in-one-linux

# Delve
EXPOSE 12345

ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/all-in-one-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]

FROM ${TARGET}