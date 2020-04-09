FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG ARCH=amd64
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14270
EXPOSE 14271
COPY ingester-linux-$ARCH /go/bin/ingester-linux
ENTRYPOINT ["/go/bin/ingester-linux"]
