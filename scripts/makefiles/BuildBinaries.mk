# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

GOBUILD_EXEC := CGO_ENABLED=0 installsuffix=cgo $(GO) build -trimpath
STYLE_BOLD_BLUE := \e[1m\e[34m
STYLE_BOLD_ORANGE := \033[1m\033[38;5;208m
STYLE_RESET := \e[39m\e[0m

# This macro expects $GOOS/$GOARCH env variables set to reflect the desired target platform.
# It also expects one argument: the name of the binary being built.
define GOBUILD
@printf "🚧 building binary '$(STYLE_BOLD_ORANGE)%s$(STYLE_RESET)' for $$(go env GOOS)-$$(go env GOARCH)\n" "$1"
$(GOBUILD_EXEC)
endef

ifeq ($(DEBUG_BINARY),)
	DISABLE_OPTIMIZATIONS =
	SUFFIX =
	TARGET = release
else
	DISABLE_OPTIMIZATIONS = -gcflags="all=-N -l"
	SUFFIX = -debug
	TARGET = debug
endif

.PHONY: build-ui
build-ui: cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/index.html.gz

cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/index.html.gz: jaeger-ui/packages/jaeger-ui/build/index.html
	# do not delete dot-files
	rm -rf cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/*
	cp -r jaeger-ui/packages/jaeger-ui/build/* cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/
	find cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual -type f | grep -v .gitignore | xargs gzip --no-name
	# copy the timestamp for index.html.gz from the original file
	touch -t $$(date -r jaeger-ui/packages/jaeger-ui/build/index.html '+%Y%m%d%H%M.%S') cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/index.html.gz
	ls -lF cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual/

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
	$(call GOBUILD,tracegen) -o ./cmd/tracegen/tracegen-$(GOOS)-$(GOARCH) ./cmd/tracegen/

.PHONY: build-anonymizer
build-anonymizer:
	$(call GOBUILD,anonymizer) -o ./cmd/anonymizer/anonymizer-$(GOOS)-$(GOARCH) ./cmd/anonymizer/

.PHONY: build-esmapping-generator
build-esmapping-generator:
	$(call GOBUILD,esmapping-generator) -o ./cmd/esmapping-generator/esmapping-generator-$(GOOS)-$(GOARCH) ./cmd/esmapping-generator/

.PHONY: build-es-index-cleaner
build-es-index-cleaner:
	$(call GOBUILD,es-index-cleaner) -o ./cmd/es-index-cleaner/es-index-cleaner-$(GOOS)-$(GOARCH) ./cmd/es-index-cleaner/

.PHONY: build-es-rollover
build-es-rollover:
	$(call GOBUILD,es-rollover) -o ./cmd/es-rollover/es-rollover-$(GOOS)-$(GOARCH) ./cmd/es-rollover/

# Requires variables: $(BIN_NAME) $(BIN_PATH) $(GO_TAGS) $(DISABLE_OPTIMIZATIONS) $(SUFFIX) $(GOOS) $(GOARCH) $(BUILD_INFO)
# Other targets can depend on this one but with a unique suffix to ensure it is always executed.
BIN_PATH = ./cmd/$(BIN_NAME)
.PHONY: _build-a-binary
_build-a-binary-%:
	$(call GOBUILD,$(BIN_PATH)) $(DISABLE_OPTIMIZATIONS) $(GO_TAGS) -o $(BIN_PATH)/$(BIN_NAME)$(SUFFIX)-$(GOOS)-$(GOARCH) $(BUILD_INFO) $(BIN_PATH)

.PHONY: build-jaeger
build-jaeger: BIN_NAME = jaeger
build-jaeger: build-ui _build-a-binary-jaeger$(SUFFIX)-$(GOOS)-$(GOARCH)
	@ set -euf -o pipefail ; \
	echo "Checking version of built binary" ; \
	REAL_GOOS=$(shell GOOS= $(GO) env GOOS) ; \
	REAL_GOARCH=$(shell GOARCH= $(GO) env GOARCH) ; \
	if [ "$(GOOS)" == "$$REAL_GOOS" ] && [ "$(GOARCH)" == "$$REAL_GOARCH" ]; then \
		./cmd/jaeger/jaeger-$(GOOS)-$(GOARCH) version 2>/dev/null ; \
		echo "" ; \
		want=$(GIT_CLOSEST_TAG) ; \
		have=$$(./cmd/jaeger/jaeger-$(GOOS)-$(GOARCH) version 2>/dev/null | jq -r .gitVersion) ; \
		if [ "$$want" == "$$have" ]; then \
			echo "☑️ versions match: want=$$want, have=$$have" ; \
		else \
			echo "❌ ERROR: version mismatch: want=$$want, have=$$have" ; \
			false; \
		fi ; \
	else \
		echo ".. skipping version check for cross-compilation" ; \
		echo ".. see build-binaries-$(GOOS)-$(GOARCH)" ; \
	fi

.PHONY: build-remote-storage
build-remote-storage: BIN_NAME = remote-storage
build-remote-storage: _build-a-binary-remote-storage$(SUFFIX)-$(GOOS)-$(GOARCH)

# build all binaries for the current platform
.PHONY: build-binaries
build-binaries: _build-platform-binaries

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
		build-remote-storage \
		build-examples \
		build-tracegen \
		build-anonymizer \
		build-esmapping-generator \
		build-es-index-cleaner \
		build-es-rollover
# invoke make recursively such that DEBUG_BINARY=1 can take effect
# skip debug builds if SKIP_DEBUG_BINARIES is set to 1 (e.g., during PRs to save CI time)
ifneq ($(SKIP_DEBUG_BINARIES),1)
	$(MAKE) _build-platform-binaries-debug GOOS=$(GOOS) GOARCH=$(GOARCH)
endif

# build binaries that support DEBUG release, for one specific platform GOOS/GOARCH
.PHONY: _build-platform-binaries-debug
_build-platform-binaries-debug: DEBUG_BINARY=1
_build-platform-binaries-debug: \
	build-jaeger \
	build-remote-storage

.PHONY: build-all-platforms
build-all-platforms:
	for platform in $$(echo "$(PLATFORMS)" | tr ',' ' ' | tr '/' '-'); do \
	  echo "Building binaries for $$platform"; \
	  $(MAKE) build-binaries-$$platform; \
	done
