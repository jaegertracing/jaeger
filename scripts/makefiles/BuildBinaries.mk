# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This command expects $GOOS/$GOARCH env variables set to reflect the desired target platform.
GOBUILD=echo "building binary for $$(go env GOOS)-$$(go env GOARCH)"; \
  CGO_ENABLED=0 installsuffix=cgo $(GO) build -trimpath

ifeq ($(DEBUG_BINARY),)
	DISABLE_OPTIMIZATIONS =
	SUFFIX =
	TARGET = release
else
	DISABLE_OPTIMIZATIONS = -gcflags="all=-N -l"
	SUFFIX = -debug
	TARGET = debug
endif

build-ui: cmd/query/app/ui/actual/index.html.gz

cmd/query/app/ui/actual/index.html.gz: jaeger-ui/packages/jaeger-ui/build/index.html
	# do not delete dot-files
	rm -rf cmd/query/app/ui/actual/*
	cp -r jaeger-ui/packages/jaeger-ui/build/* cmd/query/app/ui/actual/
	find cmd/query/app/ui/actual -type f | grep -v .gitignore | xargs gzip --no-name
	# copy the timestamp for index.html.gz from the original file
	touch -t $$(date -r jaeger-ui/packages/jaeger-ui/build/index.html '+%Y%m%d%H%M.%S') cmd/query/app/ui/actual/index.html.gz
	ls -lF cmd/query/app/ui/actual/

jaeger-ui/packages/jaeger-ui/build/index.html:
	$(MAKE) rebuild-ui

.PHONY: rebuild-ui
rebuild-ui:
	@echo "::group::rebuild-ui logs"
	bash ./scripts/build/rebuild-ui.sh
	@echo "NOTE: This target only rebuilds the UI assets inside jaeger-ui/packages/jaeger-ui/build/."
	@echo "NOTE: To make them usable from query-service run 'make build-ui'."
	@echo "::endgroup::"

.PHONY: build-examples
build-examples:
	$(GOBUILD) -o ./examples/hotrod/hotrod-$(GOOS)-$(GOARCH) ./examples/hotrod/main.go

.PHONY: build-tracegen
build-tracegen:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/tracegen/tracegen-$(GOOS)-$(GOARCH) ./cmd/tracegen/

.PHONY: build-anonymizer
build-anonymizer:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/anonymizer/anonymizer-$(GOOS)-$(GOARCH) ./cmd/anonymizer/

.PHONY: build-esmapping-generator
build-esmapping-generator:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/esmapping-generator/esmapping-generator-$(GOOS)-$(GOARCH) ./cmd/esmapping-generator/

.PHONY: build-es-index-cleaner
build-es-index-cleaner:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/es-index-cleaner/es-index-cleaner-$(GOOS)-$(GOARCH) ./cmd/es-index-cleaner/

.PHONY: build-es-rollover
build-es-rollover:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/es-rollover/es-rollover-$(GOOS)-$(GOARCH) ./cmd/es-rollover/

# Requires variables: $(BIN_NAME) $(BIN_PATH) $(GO_TAGS) $(DISABLE_OPTIMIZATIONS) $(SUFFIX) $(GOOS) $(GOARCH) $(BUILD_INFO)
# Other targets can depend on this one but with a unique suffix to ensure it is always executed.
BIN_PATH = ./cmd/$(BIN_NAME)
.PHONY: _build-a-binary
_build-a-binary-%:
	$(GOBUILD) $(DISABLE_OPTIMIZATIONS) $(GO_TAGS) -o $(BIN_PATH)/$(BIN_NAME)$(SUFFIX)-$(GOOS)-$(GOARCH) $(BUILD_INFO) $(BIN_PATH)

.PHONY: build-jaeger
build-jaeger: BIN_NAME = jaeger
build-jaeger: BUILD_INFO = $(BUILD_INFO_V2)
build-jaeger: build-ui _build-a-binary-jaeger$(SUFFIX)-$(GOOS)-$(GOARCH)
	# perform version check when not cross-compiling for a different platform
	@ if [ "$(GOOS)" = "$(shell go env GOOS)" ] && [ "$(GOARCH)" = "$(shell go env GOARCH)" ]; then \
		./cmd/jaeger/jaeger-$(GOOS)-$(GOARCH) version ; \
		echo GIT_CLOSEST_TAG_V2 = $(GIT_CLOSEST_TAG_V2); \
		want=$(GIT_CLOSEST_TAG_V2) ; \
		have=$$(./cmd/jaeger/jaeger-$(GOOS)-$(GOARCH) version 2>/dev/null | jq -r .gitVersion) ; \
		echo want = $$want have = $$have; \
		if [ "$$want" != "$$have" ]; then \
		echo "❌ ERROR: version mismatch: want=$$want, have=$$have" ; \
		false; \
		fi ; \
	fi

.PHONY: build-all-in-one
build-all-in-one: BIN_NAME = all-in-one
build-all-in-one: build-ui _build-a-binary-all-in-one$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-query
build-query: BIN_NAME = query
build-query: build-ui _build-a-binary-query$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-collector
build-collector: BIN_NAME = collector
build-collector: _build-a-binary-collector$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-ingester
build-ingester: BIN_NAME = ingester
build-ingester: _build-a-binary-ingester$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-remote-storage
build-remote-storage: BIN_NAME = remote-storage
build-remote-storage: _build-a-binary-remote-storage$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-binaries-linux-amd64
build-binaries-linux-amd64:
	GOOS=linux GOARCH=amd64 $(MAKE) _build-platform-binaries

# helper sysp targets are defined in Makefile.Windows.mk
.PHONY: build-binaries-windows-amd64
build-binaries-windows-amd64:
	$(MAKE) _build-syso
	GOOS=windows GOARCH=amd64 $(MAKE) _build-platform-binaries
	$(MAKE) _clean-syso

.PHONY: build-binaries-darwin-amd64
build-binaries-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-darwin-arm64
build-binaries-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-linux-s390x
build-binaries-linux-s390x:
	GOOS=linux GOARCH=s390x $(MAKE) _build-platform-binaries

.PHONY: build-binaries-linux-arm64
build-binaries-linux-arm64:
	GOOS=linux GOARCH=arm64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-linux-ppc64le
build-binaries-linux-ppc64le:
	GOOS=linux GOARCH=ppc64le $(MAKE) _build-platform-binaries

# build all binaries for one specific platform GOOS/GOARCH
.PHONY: _build-platform-binaries
_build-platform-binaries: \
		build-jaeger \
		build-all-in-one \
		build-collector \
		build-query \
		build-ingester \
		build-remote-storage \
		build-examples \
		build-tracegen \
		build-anonymizer \
		build-esmapping-generator \
		build-es-index-cleaner \
		build-es-rollover
# invoke make recursively such that DEBUG_BINARY=1 can take effect
	$(MAKE) _build-platform-binaries-debug GOOS=$(GOOS) GOARCH=$(GOARCH) DEBUG_BINARY=1

# build binaries that support DEBUG release, for one specific platform GOOS/GOARCH
.PHONY: _build-platform-binaries-debug
_build-platform-binaries-debug:
_build-platform-binaries-debug: \
	build-jaeger \
	build-collector \
	build-query \
	build-ingester \
	build-remote-storage \
	build-all-in-one

.PHONY: build-all-platforms
build-all-platforms:
	for platform in $$(echo "$(PLATFORMS)" | tr ',' ' ' | tr '/' '-'); do \
	  echo "Building binaries for $$platform"; \
	  $(MAKE) build-binaries-$$platform; \
	done
