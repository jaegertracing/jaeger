FROM scratch

# Agent zipkin.thrift compact
EXPOSE 5775/udp

# Agent jaeger.thrift compact
EXPOSE 6831/udp

# Agent jaeger.thrift binary
EXPOSE 6832/udp

# Agent config HTTP
EXPOSE 5778

# Collector HTTP
EXPOSE 14268

# Collector gRPC
EXPOSE 14270

# Web HTTP
EXPOSE 16686

COPY ./cmd/standalone/standalone-linux /go/bin/
COPY ./cmd/standalone/sampling_strategies.json /etc/jaeger/

ENTRYPOINT ["/go/bin/standalone-linux"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]
