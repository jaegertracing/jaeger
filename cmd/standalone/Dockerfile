FROM golang:1.7
EXPOSE 5775 6831 6832 14267 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/

ENTRYPOINT /go/bin/standalone-linux --span-storage.type=memory --query.static-files=/go/src/jaeger-ui-build/build/
