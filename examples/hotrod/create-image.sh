cd $GOPATH/src/github.com/jaegertracing/jaeger
make install
cd $GOPATH/src/github.com/jaegertracing/jaeger/examples/hotrod
CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o hotrod-linux main.go
docker build -t jaegertracing/hotrod -f Dockerfile .
