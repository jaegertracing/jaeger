FROM scratch

RUN apk add --no-cache bash

EXPOSE 5775/udp 6831/udp 6832/udp 5778
COPY ./certs/server-cert.pem /go/bin/
COPY ./cmd/agent/agent-linux /go/bin/
ENTRYPOINT ["/go/bin/agent-linux", "--reporter.type=grpc", "--reporter.grpc.host-port=a1bdf787a1ae411e9b8e8067363d5e3e-10dd980f75d67542.elb.ap-northeast-1.amazonaws.com:14250", "--reporter.grpc.tls.cert=/go/bin/server-cert.pem"]
