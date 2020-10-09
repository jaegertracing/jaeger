FROM golang:1.15.2-alpine AS base
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git && \
    go get github.com/go-delve/delve/cmd/dlv && \
    cd /go/src/github.com/go-delve/delve && \
    make install && \
    rm -rf /var/cache/apk/*

FROM alpine:latest AS release
ARG TARGETARCH=amd64
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY query-linux-$TARGETARCH /go/bin/query-linux
EXPOSE 16686/tcp
ENTRYPOINT ["/go/bin/query-linux"]

FROM golang:1.15.2-alpine AS debug
ARG TARGETARCH=amd64
COPY --from=base /go/bin/dlv /go/bin/dlv
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY query-linux-$TARGETARCH /go/bin/query-linux
EXPOSE 12345/tcp 16686/tcp
CMD ["/go/bin/dlv", "exec", "/go/bin/query-linux", "--headless", "--listen=:12345", "--api-version=2", "--log"]
