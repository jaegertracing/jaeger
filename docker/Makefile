VERSION := 1.0.0
ROOT_IMAGE ?= alpine:3.13
CERT_IMAGE := $(ROOT_IMAGE)
GOLANG_IMAGE := golang:1.15-alpine

BASE_IMAGE := localhost:5000/baseimg_alpine:latest
DEBUG_IMAGE := localhost:5000/debugimg_alpine:latest
PLATFORMS := linux/amd64,linux/s390x,linux/arm64

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
