FROM scratch
ARG ARCH=amd64

COPY tracegen-linux-$ARCH /go/bin/tracegen-linux
ENTRYPOINT ["/go/bin/tracegen-linux"]
