FROM scratch

EXPOSE 5775/udp 6831/udp 6832/udp 5778
COPY ./certs/server-cert.pem /go/bin/
COPY ./cmd/agent/agent-linux /go/bin/
ENTRYPOINT ["/go/bin/agent-linux"]