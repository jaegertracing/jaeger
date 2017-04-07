FROM golang:1.7
EXPOSE 5775 6831 6832 14267 3000 16686 8080 8081 8082 8083

COPY ./examples/hotrod/hotrod-demo-linux /bin/
COPY ./cmd/standalone/standalone-linux /bin/

ENTRYPOINT /bin/hotrod-demo-linux all & /bin/standalone-linux --span-storage.type=memory

#example build command from github.com/uber/jaeger directory: docker build -f cmd/standalone/Dockerfile -t standalone .
#example docker run command: docker run --net=host --publish 5755:5775 --publish 6831:6832 --publish 14267:14267 --publish 16686:16686 --publish 3000:3000 --publish 8000:8003 --name standalone --rm standalone