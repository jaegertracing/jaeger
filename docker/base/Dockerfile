FROM alpine:3.19.1 AS cert
RUN apk add --update --no-cache ca-certificates mailcap

FROM alpine:3.19.1
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=cert /etc/mime.types /etc/mime.types
