FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14267
EXPOSE 14250

COPY ./certs/server-cert.pem /go/bin/
COPY ./certs/server-key.pem /go/bin/
COPY ./cmd/collector/collector-linux /go/bin/

ENTRYPOINT ["/go/bin/collector-linux"]
