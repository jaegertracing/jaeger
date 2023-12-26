# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SWAGGER_VER=0.30.5
SWAGGER_IMAGE=quay.io/goswagger/swagger:v$(SWAGGER_VER)
SWAGGER=docker run --rm -it -u ${shell id -u} -v "${PWD}:/go/src/" -w /go/src/ $(SWAGGER_IMAGE)
SWAGGER_GEN_DIR=swagger-gen
SWAGGER_ZIPKIN_IDL=./idl/swagger/zipkin2-api.yaml

.PHONY: swagger-zipkin
swagger-zipkin: init-submodules
	@#curl -sSL -o ./swagger-gen/zipkin2-api.yaml https://zipkin.io/zipkin-api/zipkin2-api.yaml
	$(SWAGGER) generate server -f $(SWAGGER_ZIPKIN_IDL) -t $(SWAGGER_GEN_DIR) -O PostSpans --exclude-main
	rm -f \
		$(SWAGGER_GEN_DIR)/restapi/operations/post_spans_urlbuilder.go \
		$(SWAGGER_GEN_DIR)/restapi/server.go \
		$(SWAGGER_GEN_DIR)/restapi/configure_zipkin.go \
		$(SWAGGER_GEN_DIR)/models/trace.go \
		$(SWAGGER_GEN_DIR)/models/list_of_traces.go \
		$(SWAGGER_GEN_DIR)/models/dependency_link.go
