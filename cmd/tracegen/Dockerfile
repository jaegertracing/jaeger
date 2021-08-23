FROM scratch
ARG TARGETARCH

COPY tracegen-linux-$TARGETARCH /go/bin/tracegen-linux
ENTRYPOINT ["/go/bin/tracegen-linux"]
