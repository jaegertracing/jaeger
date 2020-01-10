FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

EXPOSE 14270
EXPOSE 14271
COPY ingester-linux /go/bin/
ENTRYPOINT ["/go/bin/ingester-linux"]
