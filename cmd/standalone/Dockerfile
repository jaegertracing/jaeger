FROM golang:1.7
# expose agent UDP ports and query port
EXPOSE 5775/udp 6831/udp 6832/udp 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/

ENTRYPOINT /go/bin/standalone-linux --span-storage.type=memory --query.static-files=/go/src/jaeger-ui-build/build/
