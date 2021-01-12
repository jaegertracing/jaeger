ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64
COPY ingester-linux-$TARGETARCH /go/bin/ingester-linux
EXPOSE 14270/tcp 14271/tcp
ENTRYPOINT ["/go/bin/ingester-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY ingester-debug-linux-$TARGETARCH /go/bin/ingester-linux
EXPOSE 12345/tcp 14270/tcp 14271/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/ingester-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
