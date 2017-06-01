FROM scratch

# Agent zipkin.thrift
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Sampling processor
EXPOSE 5778

# Collector HTTP
EXPOSE 14268

# Web HTTP
EXPOSE 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/

CMD ["/go/bin/standalone-linux","--span-storage.type=memory","--query.static-files=/go/src/jaeger-ui-build/build/"]
