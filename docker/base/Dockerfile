ARG cert_image
ARG root_image

FROM $cert_image AS cert
RUN apk add --update --no-cache ca-certificates

FROM $root_image
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
