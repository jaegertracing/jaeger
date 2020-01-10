FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

EXPOSE 16686
COPY query-linux /go/bin/

ENTRYPOINT ["/go/bin/query-linux"]
