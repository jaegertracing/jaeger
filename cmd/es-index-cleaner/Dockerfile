ARG base_image

FROM $base_image AS release
ARG TARGETARCH
COPY es-index-cleaner-linux-$TARGETARCH /go/bin/es-index-cleaner-linux
ENTRYPOINT ["/go/bin/es-index-cleaner-linux"]
