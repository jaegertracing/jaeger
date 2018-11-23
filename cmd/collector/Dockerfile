FROM jaegertracing/jaeger-service-base
EXPOSE 14267
EXPOSE 14250
COPY collector-linux /go/bin/
ENTRYPOINT ["/go/bin/collector-linux"]
