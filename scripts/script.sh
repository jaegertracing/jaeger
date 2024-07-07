./scripts/build-upload-docker-images.sh 
./scripts/es-integration-test.sh elasticsearch 7.x v1
bash scripts/build-upload-a-docker-image.sh  -b -c jaeger-es-index-cleaner -d cmd/es-index-cleaner -p linux/amd64  -t release
docker buildx build \
  --build-arg base_image=localhost:5000/baseimg_alpine:latest \
  --platform=linux/amd64 \
  --file cmd/es-index-cleaner/Dockerfile \
  -t jaegertracing/jaeger-es-index-cleaner:local-test \
  cmd/es-index-cleaner
GO=go
GOARCH ?= $(shell $(GO) env GOARCH)
echo GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-es-index-cleaner
docker build \
  --build-arg base_image=localhost:5000/baseimg_alpine:latest \
  --file cmd/es-index-cleaner/Dockerfile \
  -t jaegertracing/jaeger-es-index-cleaner:local-test \
  cmd/es-index-cleaner


