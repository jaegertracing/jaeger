FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14267
COPY ingester-linux /go/bin/
ENTRYPOINT ["/go/bin/ingester-linux"]
