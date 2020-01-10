FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

EXPOSE 8080 8081 8082 8083
COPY hotrod-linux /go/bin/
ENTRYPOINT ["/go/bin/hotrod-linux"]
CMD ["all"]
