ARG base_image

FROM $base_image AS release
ARG TARGETARCH
COPY es-rollover-linux-$TARGETARCH /go/bin/es-rollover
ENTRYPOINT ["/go/bin/es-rollover"]
