FROM golang:1.7

ADD . /go/src/github.com/uber/jaeger

ENTRYPOINT go run /go/src/github.com/uber/jaeger/cmd/standalone/main.go --span-storage.type=memory

EXPOSE 5775 6831 6832 14267 16686

#example build command from github.com/uber/jaeger directory: docker build -f cmd/standalone/Dockerfile -t standalone .
#example docker run command: docker run --publish 5755:5775 --publish 6831:6831 --publish 6832:6832 --publish 14267:14267 --publish 16686:16686 --name standalone --rm standalone