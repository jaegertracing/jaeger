FROM scratch
ARG TARGETARCH

COPY crossdock-linux-arm64 /go/bin/crossdock-linux

EXPOSE 8080
ENTRYPOINT ["/go/bin/crossdock-linux"]
