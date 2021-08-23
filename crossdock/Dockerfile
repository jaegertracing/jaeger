FROM scratch
ARG TARGETARCH

COPY crossdock-linux-$TARGETARCH /go/bin/crossdock-linux

EXPOSE 8080
ENTRYPOINT ["/go/bin/crossdock-linux"]
