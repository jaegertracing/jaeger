FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG ARCH=amd64

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY opentelemetry-agent-linux-$ARCH /go/bin/opentelemetry-agent-linux
ENTRYPOINT ["/go/bin/opentelemetry-agent-linux"]
