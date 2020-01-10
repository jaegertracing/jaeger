FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

EXPOSE 14267
EXPOSE 14250
COPY collector-linux /go/bin/
ENTRYPOINT ["/go/bin/collector-linux"]
