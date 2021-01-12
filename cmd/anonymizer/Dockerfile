FROM scratch
ARG TARGETARCH=amd64

COPY anonymizer-linux-$TARGETARCH /go/bin/anonymizer-linux
ENTRYPOINT ["/go/bin/anonymizer-linux"]
