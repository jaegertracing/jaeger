FROM golang:1.15.2-alpine AS base
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git && \
    go get github.com/go-delve/delve/cmd/dlv && \
    cd /go/src/github.com/go-delve/delve && \
    make install && \
    rm -rf /var/cache/apk/*

FROM alpine:latest AS release
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY agent-linux-$TARGETARCH /go/bin/agent-linux
EXPOSE 5775/udp 6831/udp 6832/udp 5778/tcp
ENTRYPOINT ["/go/bin/agent-linux"]
USER ${USER_UID}

FROM golang:1.15.2-alpine AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY --from=base /go/bin/dlv /go/bin/dlv
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY agent-debug-linux-$TARGETARCH /go/bin/agent-linux
EXPOSE 5775/udp 6831/udp 6832/udp 5778/tcp 12345/tcp
CMD ["/go/bin/dlv", "exec", "/go/bin/agent-linux", "--headless", "--listen=:12345", "--api-version=2", "--log"]
USER ${USER_UID}
