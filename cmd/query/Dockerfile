FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG ARCH=amd64

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 16686
COPY query-linux-$ARCH /go/bin/query-linux

ENTRYPOINT ["/go/bin/query-linux"]
