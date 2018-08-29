FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 16686
COPY query-linux /go/bin/

ENTRYPOINT ["/go/bin/query-linux", "--cassandra.keyspace=jaeger_v1_dc1", "--cassandra.servers=cassandra"]
