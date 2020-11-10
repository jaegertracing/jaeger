ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64
COPY query-linux-$TARGETARCH /go/bin/query-linux
EXPOSE 16686/tcp
ENTRYPOINT ["/go/bin/query-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY query-debug-linux-$TARGETARCH /go/bin/query-linux
EXPOSE 12345/tcp 16686/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/query-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
