# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Generate gogo, swagger, go-validators, gRPC-storage-plugin output.
#
# -I declares import folders, in order of importance. This is how proto resolves the protofile imports.
# It will check for the protofile relative to each of thesefolders and use the first one it finds.
#
# --gogo_out generates GoGo Protobuf output with gRPC plugin enabled.
# --govalidators_out generates Go validation files for our messages types, if specified.
#
# The lines starting with Mgoogle/... are proto import replacements,
# which cause the generated file to import the specified packages
# instead of the go_package's declared by the imported protof files.
#

DOCKER_PROTOBUF_VERSION=0.5.0
DOCKER_PROTOBUF=jaegertracing/protobuf:$(DOCKER_PROTOBUF_VERSION)
PROTOC := docker run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD} ${DOCKER_PROTOBUF} --proto_path=${PWD}

PATCHED_OTEL_PROTO_DIR = proto-gen/.patched-otel-proto

PROTO_INCLUDES := \
	-Iidl/proto/api_v2 \
	-Iidl/proto/api_v3 \
	-Imodel/proto/metrics \
	-I/usr/include/github.com/gogo/protobuf

# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api \
		Mmodel.proto=github.com/jaegertracing/jaeger/model \
	| $(SED) 's/  */,/g')

OPENMETRICS_PROTO_FILES=$(wildcard model/proto/metrics/*.proto)

# The source directory for OTLP Protobufs from the sub-sub-module.
OTEL_PROTO_SRC_DIR=idl/opentelemetry-proto/opentelemetry/proto

# Find all OTEL .proto files, remove leading path (only keep relevant namespace dirs).
OTEL_PROTO_FILES=$(subst $(OTEL_PROTO_SRC_DIR)/,,\
   $(shell ls $(OTEL_PROTO_SRC_DIR)/{common,resource,trace}/v1/*.proto))

# Macro to execute a command passed as argument.
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define exec-command
$(1)

endef

# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define print_caption
  @echo "🏗️ "
  @echo "🏗️ " $1
  @echo "🏗️ "

endef

# Macro to compile Protobuf $(2) into directory $(1). $(3) can provide additional flags.
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
# Arguments:
#  $(1) - output directory
#  $(2) - path to the .proto file
#  $(3) - additional flags to pass to protoc, e.g. extra -Ixxx
#  $(4) - additional options to pass to gogo plugin
define proto_compile
  $(call print_caption, "Processing $(2) --> $(1)")

  $(PROTOC) \
    $(PROTO_INCLUDES) \
    --gogo_out=plugins=grpc,$(strip $(4)),$(PROTO_GOGO_MAPPINGS):$(PWD)/$(strip $(1)) \
    $(3) $(2)

endef

# TODO add proto-hotrod to the list after regenerating its file (may need linter tweaking)
.PHONY: proto
proto: proto-model \
	proto-api-v2 \
	proto-storage-v1 \
	proto-hotrod \
	proto-zipkin \
	proto-openmetrics \
	proto-otel \
	proto-api-v3

.PHONY: proto-model
proto-model:
	$(call proto_compile, model, idl/proto/api_v2/model.proto)
	$(PROTOC) -Imodel/proto --go_out=$(PWD)/model/ model/proto/model_test.proto

.PHONY: proto-api-v2
proto-api-v2:
	$(call proto_compile, proto-gen/api_v2, idl/proto/api_v2/query.proto)
	$(call proto_compile, proto-gen/api_v2, idl/proto/api_v2/collector.proto)
	$(call proto_compile, proto-gen/api_v2, idl/proto/api_v2/sampling.proto)

.PHONY: proto-openmetrics
proto-openmetrics:
	$(call print_caption, Processing OpenMetrics Protos)
	$(foreach file,$(OPENMETRICS_PROTO_FILES),$(call proto_compile, proto-gen/api_v2/metrics, $(file)))
	@# TODO why is this file included in model/proto/metrics/ in the first place?
	rm proto-gen/api_v2/metrics/otelmetric.pb.go

.PHONY: proto-storage-v1
proto-storage-v1:
	$(call proto_compile, proto-gen/storage_v1, plugin/storage/grpc/proto/storage.proto, -Iplugin/storage/grpc/proto)
	$(PROTOC) \
		-Iplugin/storage/grpc/proto \
		--go_out=$(PWD)/plugin/storage/grpc/proto/ \
		plugin/storage/grpc/proto/storage_test.proto

.PHONY: proto-hotrod
proto-hotrod:
	$(call proto_compile, , examples/hotrod/services/driver/driver.proto)

.PHONY: proto-zipkin
proto-zipkin:
	$(call proto_compile, proto-gen/zipkin, idl/proto/zipkin.proto, -Iidl/proto)

# Target 'proto-prepare-otel' modifies OTEL proto to use proto-import path jaeger.proto.*
# The modification is needed because OTEL collector already uses opentelemetry.proto.*
# and two compiled protobuf types cannot have the same import path. The root cause is that the compiled OTLP
# in the collector is in private package, hence it cannot be used in Jaeger.
.PHONY: proto-prepare-otel
proto-prepare-otel:
	$(call print_caption, Enriching OpenTelemetry Protos into $(PATCHED_OTEL_PROTO_DIR))

	rm -rf $(PATCHED_OTEL_PROTO_DIR)
	mkdir -p $(PATCHED_OTEL_PROTO_DIR)

	@# TODO replace otel_proto_patch.sed below with otel/collector/proto_patch.sed to include gogo annotations.
	@$(foreach file,$(OTEL_PROTO_FILES), \
	   $(call exec-command,\
	     echo $(file); \
		 mkdir -p $(shell dirname $(PATCHED_OTEL_PROTO_DIR)/$(file)); \
		 $(SED) -f otel_proto_patch.sed $(OTEL_PROTO_SRC_DIR)/$(file) > $(PATCHED_OTEL_PROTO_DIR)/$(file)))

# Target 'proto-otel' generates classes for OpenTelemetry OTLP format. We cannot reuse similar classes
# already generated by OTel Collector because those are private / internal.
.PHONY: proto-otel
proto-otel: proto-prepare-otel
	$(foreach file,$(OTEL_PROTO_FILES), \
	  $(call proto_compile, proto-gen/otel, $(file), -I$(PATCHED_OTEL_PROTO_DIR), paths=source_relative))

# Similar to 'proto-prepare-otel', this target modifies OTEL Protos by changing their Go package.
# This way the API v3 service uses official OTEL types like opentelemetry.proto.trace.v1.ResourceSpans
# which at runtime are mapped to our internal classes generated in proto-gen/otel by 'proto-otel' target.
.PHONY: proto-api-v3
proto-api-v3:
	$(call print_caption, Enriching OpenTelemetry Protos into $(PATCHED_OTEL_PROTO_DIR))
	rm -rf $(PATCHED_OTEL_PROTO_DIR)/*
	cp -R idl/opentelemetry-proto/* $(PATCHED_OTEL_PROTO_DIR)
	find $(PATCHED_OTEL_PROTO_DIR) -name "*.proto" | xargs -L 1 $(SED) -i 's+go.opentelemetry.io/proto/otlp+github.com/jaegertracing/jaeger/proto-gen/otel+g'

	$(call proto_compile, proto-gen/api_v3, idl/proto/api_v3/query_service.proto, -I$(PATCHED_OTEL_PROTO_DIR))

	$(call print_caption, Generate API v3 gRPC Gateway)
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		-I$(PATCHED_OTEL_PROTO_DIR) \
 		--grpc-gateway_out=logtostderr=true,grpc_api_configuration=idl/proto/api_v3/query_service_http.yaml,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen/api_v3 \
		idl/proto/api_v3/query_service.proto
