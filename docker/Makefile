VERSION := 1.0.0
ROOT_IMAGE ?= alpine:3.13
CERT_IMAGE := $(ROOT_IMAGE)
GOLANG_IMAGE := golang:1.16-alpine

BASE_IMAGE := localhost:5000/baseimg_alpine:latest
DEBUG_IMAGE := localhost:5000/debugimg_alpine:latest
PLATFORMS := linux/amd64,linux/s390x,linux/ppc64le,linux/arm64

create-baseimg-debugimg: create-baseimg create-debugimg

create-baseimg:
	docker buildx build -t $(BASE_IMAGE) --push \
		--build-arg root_image=$(ROOT_IMAGE) \
		--build-arg cert_image=$(CERT_IMAGE) \
		--platform=$(PLATFORMS) \
		docker/base

create-debugimg:
	docker buildx build -t $(DEBUG_IMAGE) --push \
		--build-arg golang_image=$(GOLANG_IMAGE) \
		--platform=$(PLATFORMS) \
		docker/debug

.PHONY: prepare-docker-buildx
prepare-docker-buildx:
	 docker buildx create --use --name=jaeger-build --buildkitd-flags="--allow-insecure-entitlement security.insecure --allow-insecure-entitlement network.host" --driver-opt="network=host"
	 docker run --rm -d -p 5000:5000 --name registry registry:2

.PHONY: clean-docker-buildx
clean-docker-buildx:
	docker buildx rm jaeger-build
	docker rm -f registry
