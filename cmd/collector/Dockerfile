FROM alpine as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /usr/share/ca-certificates/ /usr/share/ca-certificates/
COPY --from=certs /etc/ssl/ /etc/ssl/

EXPOSE 14267
COPY collector-linux /go/bin/
CMD ["/go/bin/collector-linux"]
