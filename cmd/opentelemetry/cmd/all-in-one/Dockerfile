FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch
ARG TARGETARCH=amd64

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY opentelemetry-all-in-one-linux-$TARGETARCH /go/bin/opentelemetry-all-in-one-linux
ENTRYPOINT ["/go/bin/opentelemetry-all-in-one-linux"]
