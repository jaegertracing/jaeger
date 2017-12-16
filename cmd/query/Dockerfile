FROM alpine as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /usr/share/ca-certificates/ /usr/share/ca-certificates/
COPY --from=certs /etc/ssl/ /etc/ssl/

EXPOSE 16686
COPY query-linux /go/bin/
ADD jaeger-ui-build /go/jaeger-ui/
CMD ["/go/bin/query-linux", "--query.static-files=/go/jaeger-ui/"]
