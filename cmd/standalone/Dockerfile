FROM golang:1.7-alpine
EXPOSE 5775 6831 6832 14267 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/

ENTRYPOINT /go/bin/standalone-linux --span-storage.type=memory --query.static-files=/go/src/jaeger-ui-build/build/

#example build command from github.com/uber/jaeger directory: docker build -f cmd/standalone/Dockerfile -t standalone .
#example docker run command: docker run --net=host --publish 5755:5775 --publish 6831:6832 --publish 14267:14267 --publish 16686:16686 --name standalone --rm standalone