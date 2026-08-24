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

DOCKER=docker
DOCKER_PROTOBUF_VERSION=0.5.1
DOCKER_PROTOBUF=jaegertracing/protobuf:$(DOCKER_PROTOBUF_VERSION)
PROTOC := ${DOCKER} run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD} ${DOCKER_PROTOBUF} --proto_path=${PWD}

# gnostic provides openapiv3/annotations.proto needed by api_v3 proto files.
# Use '=' (lazy) so go list only runs when proto-api-v3 target is actually invoked.
GNOSTIC_DIR = $(shell go list -m -f '{{.Dir}}' github.com/google/gnostic-models)
PROTOC_WITH_GNOSTIC = ${DOCKER} run --rm -u ${shell id -u} -v${PWD}:${PWD} -v"${GNOSTIC_DIR}:/gnostic/gnostic" -w${PWD} ${DOCKER_PROTOBUF} --proto_path=${PWD}

PROTO_GEN=internal/proto-gen
PATCHED_OTEL_PROTO_DIR = $(PROTO_GEN)/.patched-otel-proto

# The filter AST of RFC 0005 lives in its own proto package, which both api_v3 and
# storage/v2 import, so that the two APIs carry the same messages instead of each
# declaring its own copy. Both of their compiles map the file to this one Go package.
# It sits under internal/proto rather than internal/proto-gen because a hand-written
# conversion to the storage filter lives beside the generated types, as api_v3 does.
EXPRESSION_ROOT=internal/proto
EXPRESSION_PATH=$(EXPRESSION_ROOT)/expression/v1

PROTO_INCLUDES := \
	-Iidl/proto/api_v2 \
	-Iinternal/proto/metrics \
	-I/usr/include/github.com/gogo/protobuf \
	-Iidl/opentelemetry-proto

# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api \
		Mmodel.proto=github.com/jaegertracing/jaeger-idl/model/v1 \
		Mgnostic/openapiv3/annotations.proto=github.com/google/gnostic-models/openapiv3 \
		Mexpression/v1/expression.proto=github.com/jaegertracing/jaeger/$(EXPRESSION_PATH) \
	| $(SED) 's/  */,/g')

OPENMETRICS_PROTO_FILES=$(wildcard internal/proto/metrics/*.proto)

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
#  $(5) - protoc command override (default: $(PROTOC))
define proto_compile
  $(call print_caption, "Processing $(2) --> $(1)")

  $(if $(5),$(5),$(PROTOC)) \
    $(PROTO_INCLUDES) \
    --gogo_out=plugins=grpc,$(strip $(4)),$(PROTO_GOGO_MAPPINGS):$(PWD)/$(strip $(1)) \
    $(3) $(2)

endef

.PHONY: proto
proto: \
	proto-expression \
	proto-storage-v2 \
	proto-hotrod \
	proto-zipkin \
	proto-openmetrics \
	proto-api-v3

EXPRESSION_PATCHED_DIR=$(PROTO_GEN)/.patched/expression/v1
EXPRESSION_PATCHED=$(EXPRESSION_PATCHED_DIR)/expression.proto

.PHONY: proto-expression
proto-expression:
	mkdir -p $(EXPRESSION_PATCHED_DIR) $(EXPRESSION_PATH)
	# A patch of its own, because patch.sed anchors the gogo options on a google/protobuf
	# import that this file does not have. The options are what give the generated type the
	# Marshal/Unmarshal/Size methods that api_v3's and storage/v2's own generated code call
	# on the filter field.
	$(SED) -f ./$(PROTO_GEN)/patch-expression.sed \
		idl/proto/expression/v1/expression.proto \
		> $(EXPRESSION_PATCHED)
	# protoc appends the file's path relative to its include root, expression/v1/, to the
	# output directory, so the output root is $(EXPRESSION_ROOT), not $(EXPRESSION_PATH).
	$(call proto_compile, $(EXPRESSION_ROOT), $(EXPRESSION_PATCHED), -I$(PROTO_GEN)/.patched -I/gnostic -I/gnostic/gnostic,, $(PROTOC_WITH_GNOSTIC))


API_V2_PATCHED_DIR=$(PROTO_GEN)/.patched/api_v2
.PHONY: patch-api-v2
patch-api-v2:
	mkdir -p $(API_V2_PATCHED_DIR)
	cp idl/proto/api_v2/collector.proto $(API_V2_PATCHED_DIR)/
	cp idl/proto/api_v2/sampling.proto $(API_V2_PATCHED_DIR)/
	$(SED) 's|jaegertracing/jaeger-idl/model/v1.|jaegertracing/jaeger/model.|g' \
		idl/proto/api_v2/query.proto \
		> $(API_V2_PATCHED_DIR)/query.proto


.PHONY: proto-openmetrics
proto-openmetrics:
	$(call print_caption, Processing OpenMetrics Protos)
	$(foreach file,$(OPENMETRICS_PROTO_FILES),$(call proto_compile, $(PROTO_GEN)/api_v2/metrics, $(file)))

STORAGE_V2_PATH=$(PROTO_GEN)/storage/v2
STORAGE_V2_PATCHED_DIR=$(PROTO_GEN)/.patched/storage_v2
STORAGE_V2_PATCHED_TRACE=$(STORAGE_V2_PATCHED_DIR)/trace_storage.proto
STORAGE_V2_PATCHED_DEPENDENCY=$(STORAGE_V2_PATCHED_DIR)/dependency_storage.proto
STORAGE_V2_PATCHED_CAPABILITIES=$(STORAGE_V2_PATCHED_DIR)/capabilities.proto

.PHONY: patch-storage-v2
patch-storage-v2:
	mkdir -p $(STORAGE_V2_PATCHED_DIR)
	$(SED) -f ./$(PROTO_GEN)/patch.sed \
		idl/proto/storage/v2/trace_storage.proto \
		> $(STORAGE_V2_PATCHED_TRACE)
	$(SED) -f ./$(PROTO_GEN)/patch.sed \
		idl/proto/storage/v2/dependency_storage.proto \
		> $(STORAGE_V2_PATCHED_DEPENDENCY)
	$(SED) -f ./$(PROTO_GEN)/patch.sed \
		idl/proto/storage/v2/capabilities.proto \
		> $(STORAGE_V2_PATCHED_CAPABILITIES)

STORAGE_V2_INCLUDES=-I$(STORAGE_V2_PATCHED_DIR) -Iinternal/storage/v2/grpc/ -I$(PROTO_GEN)/.patched -I/gnostic -I/gnostic/gnostic

.PHONY: proto-storage-v2
proto-storage-v2: patch-storage-v2 proto-expression
	$(call proto_compile, $(STORAGE_V2_PATH), $(STORAGE_V2_PATCHED_TRACE), $(STORAGE_V2_INCLUDES),, $(PROTOC_WITH_GNOSTIC))
	$(call proto_compile, $(STORAGE_V2_PATH), $(STORAGE_V2_PATCHED_DEPENDENCY), $(STORAGE_V2_INCLUDES),, $(PROTOC_WITH_GNOSTIC))
	$(call proto_compile, $(STORAGE_V2_PATH), $(STORAGE_V2_PATCHED_CAPABILITIES), $(STORAGE_V2_INCLUDES),, $(PROTOC_WITH_GNOSTIC))
	@echo "🏗️  replace first instance of OTEL import with internal type"
	$(SED) -i '0,/go.opentelemetry.io\/proto\/otlp\/trace\/v1/s|go.opentelemetry.io/proto/otlp/trace/v1|github.com/jaegertracing/jaeger/internal/jptrace|' $(STORAGE_V2_PATH)/*.pb.go
	@echo "🏗️  remove all remaining OTEL imports because we're not using any other OTLP types"
	$(SED) -i 's+^.*v1 "go.opentelemetry.io/proto/otlp/trace/v1".*$$++' $(STORAGE_V2_PATH)/*.pb.go

.PHONY: proto-hotrod
proto-hotrod:
	$(call proto_compile, , examples/hotrod/services/driver/driver.proto)

.PHONY: proto-zipkin
proto-zipkin:
	$(call proto_compile, $(PROTO_GEN)/zipkin, idl/proto/zipkin.proto, -Iidl/proto)

# The API v3 service uses official OTEL type opentelemetry.proto.trace.v1.TracesData,
# which at runtime is mapped to a custom type in cmd/jaeger/internal/extension/jaegerquery/internal/internal/api_v3/traces.go
# Unfortunately, gogoproto.customtype annotation cannot be applied to a method's return type,
# only to fields in a struct, so we use regex search/replace to swap it.
# Note that the .pb.go types must be generated into the same internal package $(API_V3_PATH)
# where a manually defined traces.go file is located.
API_V3_PATH=internal/proto/api_v3
API_V3_PATCHED_DIR=$(PROTO_GEN)/.patched/api_v3
API_V3_PATCHED=$(API_V3_PATCHED_DIR)/query_service.proto
.PHONY: patch-api-v3
patch-api-v3:
	mkdir -p $(API_V3_PATCHED_DIR)
	$(SED) -f ./$(PROTO_GEN)/patch.sed \
		idl/proto/api_v3/query_service.proto \
		> $(API_V3_PATCHED)

.PHONY: proto-api-v3
proto-api-v3: patch-api-v3 proto-expression
	$(call proto_compile, $(API_V3_PATH), $(API_V3_PATCHED), -I$(API_V3_PATCHED_DIR) -I$(PROTO_GEN)/.patched -Iidl/opentelemetry-proto -I/gnostic -I/gnostic/gnostic,, $(PROTOC_WITH_GNOSTIC))
	@echo "🏗️  replace first instance of OTEL import with internal type"
	$(SED) -i '0,/go.opentelemetry.io\/proto\/otlp\/trace\/v1/s|go.opentelemetry.io/proto/otlp/trace/v1|github.com/jaegertracing/jaeger/internal/jptrace|' $(API_V3_PATH)/query_service.pb.go
	@echo "🏗️  remove all remaining OTEL imports because we're not using any other OTLP types"
	$(SED) -i 's+^.*v1 "go.opentelemetry.io/proto/otlp/trace/v1".*$$++' $(API_V3_PATH)/query_service.pb.go
