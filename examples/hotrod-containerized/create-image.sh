cd ../hotrod && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main
cd ../
docker build -t jaegertracing/hotrod -f Dockerfile .
