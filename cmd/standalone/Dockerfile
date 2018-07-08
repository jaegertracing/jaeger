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

# Web HTTP
EXPOSE 16686

COPY ./cmd/standalone/standalone-linux /go/bin/
COPY ./cmd/standalone/sampling_strategies.json /etc/jaeger/

# TODO split cmd into ENTRYPOINT
CMD ["/go/bin/standalone-linux", "--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]
