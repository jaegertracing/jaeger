ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64
COPY proxy-linux-$TARGETARCH /go/bin/proxy-linux
EXPOSE 16786/tcp
ENTRYPOINT ["/go/bin/proxy-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY proxy-debug-linux-$TARGETARCH /go/bin/proxy-linux
EXPOSE 12345/tcp 16786/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/proxy-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
