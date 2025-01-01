# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

THRIFT_VER=0.19
THRIFT_IMG=jaegertracing/thrift:$(THRIFT_VER)
THRIFT=docker run --rm -u ${shell id -u} -v "${PWD}:/data" $(THRIFT_IMG) thrift
THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_GEN_DIR=thrift-gen

.PHONY: thrift-image
thrift-image:
	$(THRIFT) -version

.PHONY: thrift
thrift: idl/thrift/jaeger.thrift thrift-image
	[ -d $(THRIFT_GEN_DIR) ] || mkdir $(THRIFT_GEN_DIR)
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/agent.thrift
	$(SED) -i.bak 's|"zipkincore"|"$(JAEGER_IMPORT_PATH)/thrift-gen/zipkincore"|g' $(THRIFT_GEN_DIR)/agent/*.go
	$(SED) -i.bak 's|"jaeger"|"$(JAEGER_IMPORT_PATH)/thrift-gen/jaeger"|g' $(THRIFT_GEN_DIR)/agent/*.go
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/jaeger.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/sampling.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/baggage.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/zipkincore.thrift
	rm -rf thrift-gen/*/*-remote thrift-gen/*/*.bak

idl/thrift/jaeger.thrift:
	$(MAKE) init-submodules
