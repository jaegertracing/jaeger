FROM jaegertracing/jaeger-service-base
EXPOSE 16686
COPY query-linux /go/bin/
ENTRYPOINT ["/go/bin/query-linux"]
