FROM scratch
ARG ARCH=amd64
EXPOSE 8080 8081 8082 8083
COPY hotrod-linux-$ARCH /go/bin/hotrod-linux
ENTRYPOINT ["/go/bin/hotrod-linux"]
CMD ["all"]
