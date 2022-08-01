ARG base_image
ARG debug_image

ARG SVC=remote-storage

FROM $base_image AS release
ARG TARGETARCH
COPY $SVC-linux-$TARGETARCH /go/bin/$SVC-linux
EXPOSE 16686/tcp
ENTRYPOINT ["/go/bin/$SVC-linux"]

FROM $debug_image AS debug
ARG TARGETARCH=amd64
COPY $SVC-debug-linux-$TARGETARCH /go/bin/$SVC-linux
EXPOSE 12345/tcp 16686/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/$SVC-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
