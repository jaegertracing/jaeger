ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64
COPY collector-linux-$TARGETARCH /go/bin/collector-linux
EXPOSE 14250/tcp
ENTRYPOINT ["/go/bin/collector-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY collector-debug-linux-$TARGETARCH /go/bin/collector-linux
EXPOSE 12345/tcp 14250/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/collector-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
