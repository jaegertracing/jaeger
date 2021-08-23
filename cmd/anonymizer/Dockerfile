FROM scratch
ARG TARGETARCH

COPY anonymizer-linux-$TARGETARCH /go/bin/anonymizer-linux
ENTRYPOINT ["/go/bin/anonymizer-linux"]
