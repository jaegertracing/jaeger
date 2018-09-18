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

COPY ./cmd/all-in-one/all-in-one-linux /go/bin/
COPY ./cmd/all-in-one/sampling_strategies.json /etc/jaeger/

ENTRYPOINT ["/go/bin/all-in-one-linux"]
CMD ["--sampling.strategies-file=/etc/jaeger/sampling_strategies.json"]
