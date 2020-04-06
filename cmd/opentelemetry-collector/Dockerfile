FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY opentelemetry-collector-linux /go/bin/
ENTRYPOINT ["/go/bin/opentelemetry-collector-linux"]
