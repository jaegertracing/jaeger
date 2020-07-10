FROM scratch
ARG TARGETARCH=amd64

COPY tracegen-linux-$TARGETARCH /go/bin/tracegen-linux
ENTRYPOINT ["/go/bin/tracegen-linux"]
