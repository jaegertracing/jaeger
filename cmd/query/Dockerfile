FROM centos:7
EXPOSE 16686

COPY query-linux /go/bin/
ADD jaeger-ui-build /go/jaeger-ui/

CMD ["/go/bin/query-linux", "--query.static-files=/go/jaeger-ui/"]
