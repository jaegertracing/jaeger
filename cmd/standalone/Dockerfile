FROM golang:1.7
EXPOSE 5775 6831 6832 14267 3000 3001 8080 8081 8082 8083

ADD . /go/src/github.com/uber/jaeger

RUN apt-get update
RUN apt-get install -y curl
RUN curl -sL https://deb.nodesource.com/setup_6.x | bash
RUN apt-get install -y nodejs
RUN node -v
RUN npm -v

RUN git clone https://github.com/uber/jaeger-ui

RUN cd jaeger-ui && npm install

ENTRYPOINT cd jaeger-ui && npm start & go run /go/src/github.com/uber/jaeger/examples/hotrod/main.go all & go run /go/src/github.com/uber/jaeger/cmd/standalone/main.go --span-storage.type=memory --query.port=3001

#example build command from github.com/uber/jaeger directory: docker build -f cmd/standalone/Dockerfile -t standalone .
#example docker run command: docker run --net=host --publish 5755:5775 --publish 6831:6832 --publish 14267:14267 --publish 3000:3001 --publish 8000:8003 --name standalone --rm standalone