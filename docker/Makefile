VERSION := 1.0.0
ROOT_IMAGE ?= alpine:3.12
CERT_IMAGE := alpine:3.12
GOLANG_IMAGE := golang:1.15-alpine

BASE_IMAGE := $(DOCKER_NAMESPACE)/baseimg:$(VERSION)-$(shell echo $(ROOT_IMAGE) | tr : -)
DEBUG_IMAGE := $(DOCKER_NAMESPACE)/debugimg:$(VERSION)-$(shell echo $(GOLANG_IMAGE) | tr : -)

create-baseimg-debugimg: create-baseimg create-debugimg

dockerx-baseimg-debugimg: setup-buildx-builder
	$(MAKE) create-baseimg-debugimg BUILDX_PUSH=true

create-baseimg:
	$(DOCKER_BUILD) -t $(BASE_IMAGE) \
		--build-arg root_image=$(ROOT_IMAGE) \
		--build-arg cert_image=$(CERT_IMAGE) \
		docker/base

create-debugimg:
	$(DOCKER_BUILD) -t $(DEBUG_IMAGE) \
		--build-arg golang_image=$(GOLANG_IMAGE) \
		docker/debug

setup-buildx-builder:
	bash ./scripts/setup-buildx-builder.sh