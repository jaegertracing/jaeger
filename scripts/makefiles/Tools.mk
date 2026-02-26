# Copyright (c) 2024 The Jaeger Authors.
# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

TOOLS_MOD_DIR   := $(SRC_ROOT)/internal/tools
TOOLS_BIN_DIR   := $(SRC_ROOT)/.tools
TOOLS_MOD_REGEX := "\s+_\s+\".*\""
TOOLS_PKG_NAMES := $(shell grep -E $(TOOLS_MOD_REGEX) < $(TOOLS_MOD_DIR)/tools.go | tr -d " _\"")
TOOLS_BIN_NAMES := $(addprefix $(TOOLS_BIN_DIR)/, $(notdir $(shell echo $(TOOLS_PKG_NAMES) | sed 's|/v[0-9]||g')))

GOFUMPT       := $(TOOLS_BIN_DIR)/gofumpt
GOVERSIONINFO := $(TOOLS_BIN_DIR)/goversioninfo
GOVULNCHECK   := $(TOOLS_BIN_DIR)/govulncheck
LINT          := $(TOOLS_BIN_DIR)/golangci-lint
MOCKERY       := $(TOOLS_BIN_DIR)/mockery
SCHEMAGEN     := $(TOOLS_BIN_DIR)/schemagen

# this target is useful for setting up local workspace, but from CI we want to call more specific ones
.PHONY: install-tools
install-tools: $(TOOLS_BIN_NAMES)

.PHONY: install-test-tools
install-test-tools: $(LINT) $(GOFUMPT)

.PHONY: install-ci
install-ci: install-test-tools

list-internal-tools:
	@echo Third party tool modules:
	@echo $(TOOLS_PKG_NAMES) | tr ' ' '\n' | sed 's/^/- /g'
	@echo Third party tool binaries:
	@echo $(TOOLS_BIN_NAMES) | tr ' ' '\n' | sed 's/^/- /g'

$(TOOLS_BIN_DIR):
	mkdir -p $@

$(TOOLS_BIN_NAMES): $(TOOLS_BIN_DIR) $(TOOLS_MOD_DIR)/go.mod $(TOOLS_MOD_DIR)/go.sum
	cd $(TOOLS_MOD_DIR) && $(GO) build -o $@ -trimpath $(shell echo $(TOOLS_PKG_NAMES) | tr ' ' '\n' | grep $(notdir $@))
