FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG TARGETARCH=amd64
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY opentelemetry-collector-linux-$TARGETARCH /go/bin/opentelemetry-collector-linux
ENTRYPOINT ["/go/bin/opentelemetry-collector-linux"]
