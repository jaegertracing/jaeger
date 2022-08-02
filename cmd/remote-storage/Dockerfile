ARG base_image
ARG debug_image

ARG SVC=remote-storage

FROM $base_image AS release
ARG TARGETARCH
COPY remote-storage-linux-$TARGETARCH /go/bin/remote-storage-linux
EXPOSE 16686/tcp
ENTRYPOINT ["/go/bin/remote-storage-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY remote-storage-debug-linux-$TARGETARCH /go/bin/remote-storage-linux
EXPOSE 12345/tcp 16686/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/remote-storage-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
