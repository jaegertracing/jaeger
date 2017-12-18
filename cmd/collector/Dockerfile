FROM alpine as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 14267
COPY collector-linux /go/bin/
CMD ["/go/bin/collector-linux"]
