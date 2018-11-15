FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14267
EXPOSE 14250
COPY collector-linux /go/bin/
ENTRYPOINT ["/go/bin/collector-linux"]
