FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG TARGETARCH=amd64
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14250
COPY collector-linux-$TARGETARCH /go/bin/collector-linux
ENTRYPOINT ["/go/bin/collector-linux"]
