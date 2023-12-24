# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# TODO JAEGER_DOCKER_PROTOBUF=jaegertracing/protobuf:0.4.0
JAEGER_DOCKER_PROTOBUF=otel/build-protobuf:0.9.0
DOCKER_NAMESPACE?=jaegertracing
DOCKER_TAG?=latest
PROTO_INTERMEDIATE_DIR = proto-gen/.patched-otel-proto
PROTOC := docker run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD} ${JAEGER_DOCKER_PROTOBUF} --proto_path=${PWD}
PROTO_INCLUDES := \
	-Iidl/proto/api_v2 \
	-Iidl/proto/api_v3 \
	-Imodel/proto/metrics \
	-I$(PROTO_INTERMEDIATE_DIR) \
	-I/usr/include/github.com/gogo/protobuf

# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types, \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api, \
		Mmodel.proto=github.com/jaegertracing/jaeger/model \
	| sed 's/ //g')

OPENMETRICS_PROTO_FILES=$(wildcard model/proto/metrics/*.proto)

# The source directory for OTLP Protobufs from the sub-sub-module.
OTEL_PROTO_SRC_DIR=idl/opentelemetry-proto

# Find all OTEL .proto files, remove repository path (only keep relevant namespace dirs).
OTEL_PROTO_FILES=$(subst $(OTEL_PROTO_SRC_DIR)/,,\
   $(shell ls $(OTEL_PROTO_SRC_DIR)/opentelemetry/proto/{common,resource,trace}/v1/*.proto))

# Macro to compile Protobuf $(2) into directory $(1).
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define proto_compile
  $(PROTOC) \
    $(PROTO_INCLUDES) \
    --gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/$(1)/ \
    $(2)

endef

# Macro to execute a command passed as argument.
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define exec-command
$(1)

endef


.PHONY: x
x:
	@echo $(OTEL_PROTO_FILES)

.PHONY: proto
proto: proto-prepare-otel
	# Generate gogo, swagger, go-validators, gRPC-storage-plugin output.
	#
	# -I declares import folders, in order of importance
	# This is how proto resolves the protofile imports.
	# It will check for the protofile relative to each of these
	# folders and use the first one it finds.
	#
	# --gogo_out generates GoGo Protobuf output with gRPC plugin enabled.
	# --govalidators_out generates Go validation files for our messages types, if specified.
	#
	# The lines starting with Mgoogle/... are proto import replacements,
	# which cause the generated file to import the specified packages
	# instead of the go_package's declared by the imported protof files.
	#
	# $$GOPATH/src is the output directory. It is relative to the GOPATH/src directory
	# since we've specified a go_package option relative to that directory.
	#
	# model/proto/jaeger.proto is the location of the protofile we use.
	#
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/model/ \
		idl/proto/api_v2/model.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v2 \
		idl/proto/api_v2/query.proto
		### --swagger_out=allow_merge=true:$(PWD)/proto-gen/openapi/ \

	@echo "Compile OpenMetrics Protos"
	$(foreach file,$(OPENMETRICS_PROTO_FILES),$(call proto_compile, proto-gen/api_v2/metrics, $(file)))

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v2 \
		idl/proto/api_v2/collector.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v2 \
		idl/proto/api_v2/sampling.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		-Iplugin/storage/grpc/proto \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/storage_v1 \
		plugin/storage/grpc/proto/storage.proto

	$(PROTOC) \
		-Imodel/proto \
		--go_out=$(PWD)/model/ \
		model/proto/model_test.proto

	$(PROTOC) \
		-Iplugin/storage/grpc/proto \
		--go_out=$(PWD)/plugin/storage/grpc/proto/ \
		plugin/storage/grpc/proto/storage_test.proto

	$(PROTOC) \
		-Iidl/proto \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/zipkin \
		idl/proto/zipkin.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,paths=source_relative,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/otel \
		$(PROTO_INTERMEDIATE_DIR)/common/v1/common.proto
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,paths=source_relative,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/otel \
		$(PROTO_INTERMEDIATE_DIR)/resource/v1/resource.proto
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,paths=source_relative,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/otel \
		$(PROTO_INTERMEDIATE_DIR)/trace/v1/trace.proto

	# Target  proto-prepare-otel modifies OTEL proto to use import path jaeger.proto.*
	# The modification is needed because OTEL collector already uses opentelemetry.proto.*
	# and two complied protobuf types cannot have the same import path. The root cause is that the compiled OTLP
	# in the collector is in private package, hence it cannot be used in Jaeger.
	# The following statements revert changes in OTEL proto and only modify go package.
	# This way the service will use opentelemetry.proto.trace.v1.ResourceSpans but in reality at runtime
	# it uses jaeger.proto.trace.v1.ResourceSpans which is the same type in a different package which
	# prevents panic of two equal proto types.
	rm -rf $(PROTO_INTERMEDIATE_DIR)/*
	cp -R idl/opentelemetry-proto/* $(PROTO_INTERMEDIATE_DIR)
	find $(PROTO_INTERMEDIATE_DIR) -name "*.proto" | xargs -L 1 sed -i 's+go.opentelemetry.io/proto/otlp+github.com/jaegertracing/jaeger/proto-gen/otel+g'
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v3 \
		idl/proto/api_v3/query_service.proto
	$(PROTOC) \
		$(PROTO_INCLUDES) \
 		--grpc-gateway_out=logtostderr=true,grpc_api_configuration=idl/proto/api_v3/query_service_http.yaml,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v3 \
		idl/proto/api_v3/query_service.proto
	rm -rf $(PROTO_INTERMEDIATE_DIR)

.PHONY: proto-prepare-otel
proto-prepare-otel:
	@echo --
	@echo -- "Enriching OpenTelemetry Protos into $(PROTO_INTERMEDIATE_DIR)"
	@echo --

	mkdir -p $(PROTO_INTERMEDIATE_DIR)
	rm -rf $(PROTO_INTERMEDIATE_DIR)/*

	@# TODO replace otel_proto_patch.sed below with otel/collector/proto_patch.sed to include gogo annotations.
	@$(foreach file,$(OTEL_PROTO_FILES), \
	   $(call exec-command,\
	     echo $(file); \
		 mkdir -p $(shell dirname $(PROTO_INTERMEDIATE_DIR)/$(file)); \
		 sed -f otel_proto_patch.sed $(OTEL_PROTO_SRC_DIR)/$(file) > $(PROTO_INTERMEDIATE_DIR)/$(file)))

.PHONY: proto-hotrod
proto-hotrod:
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/ \
		examples/hotrod/services/driver/driver.proto
