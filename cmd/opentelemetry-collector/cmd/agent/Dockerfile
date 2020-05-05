FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY opentelemetry-agent-linux /go/bin/
ENTRYPOINT ["/go/bin/opentelemetry-agent-linux"]
