# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

STORAGE_PKGS = ./internal/storage/integration/...
JAEGER_V2_STORAGE_PKGS = ./cmd/jaeger/internal/integration
INTEGRATION_TEST_FLAGS = --format standard-verbose --format-icons hivis

.PHONY: all-in-one-integration-test
all-in-one-integration-test: $(GOTESTSUM)
	TEST_MODE=integration $(GOTESTSUM) $(GOTESTSUM_FLAGS) -- $(RACE) ./cmd/jaeger/internal/all_in_one_test.go

JAEGER_MAIN_INSTALL_DIR = /tmp/jaeger-main
export JAEGER_MAIN_INSTALL_DIR

# Installs the @main jaeger binary into JAEGER_MAIN_INSTALL_DIR.
# Reusable by other backward-compatibility targets (e.g. for future backends).
.PHONY: install-jaeger-main
install-jaeger-main:
	mkdir -p $(JAEGER_MAIN_INSTALL_DIR)
	rm -rf $(JAEGER_MAIN_INSTALL_DIR)/jaeger-repo
	git clone --depth 1 --branch main https://github.com/jaegertracing/jaeger.git $(JAEGER_MAIN_INSTALL_DIR)/jaeger-repo
	(cd $(JAEGER_MAIN_INSTALL_DIR)/jaeger-repo/cmd/jaeger && go build -o $(JAEGER_MAIN_INSTALL_DIR)/jaeger .)
	rm -rf $(JAEGER_MAIN_INSTALL_DIR)/jaeger-repo

BACKWARD_COMPATIBILITY ?= false
ifeq ($(BACKWARD_COMPATIBILITY),true)
PRE_TEST := install-jaeger-main
EXTRA_TEST_ARGS := -run ".*BackwardCompatibility"
endif

# A general integration tests for jaeger-v2 storage backends,
# these tests placed at `./cmd/jaeger/internal/integration/*_test.go`.
# The integration tests are filtered by STORAGE env.
.PHONY: jaeger-v2-storage-integration-test
jaeger-v2-storage-integration-test: $(GOTESTSUM) $(PRE_TEST)
	(cd cmd/jaeger/ && go build .)
	# Expire tests results for jaeger storage integration tests since the environment
	# might have changed even though the code remains the same.
	go clean -testcache
	$(GOTESTSUM) $(INTEGRATION_TEST_FLAGS) -- $(RACE) $(EXTRA_TEST_ARGS) -coverpkg=./... -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS)

.PHONY: storage-integration-test
storage-integration-test: $(GOTESTSUM)
ifndef STORAGE
	$(error STORAGE environment variable must be set, e.g. elasticsearch, opensearch, badger, grpc)
endif
	# Expire tests results for storage integration tests since the environment might change
	# even though the code remains the same.
	go clean -testcache
	$(GOTESTSUM) $(INTEGRATION_TEST_FLAGS) -- $(RACE) -coverpkg=./... -coverprofile $(COVEROUT) $(STORAGE_PKGS)

.PHONY: badger-storage-integration-test
badger-storage-integration-test:
	STORAGE=badger $(MAKE) storage-integration-test

.PHONY: grpc-storage-integration-test
grpc-storage-integration-test:
	STORAGE=grpc $(MAKE) storage-integration-test

.PHONY: tail-sampling-integration-test
tail-sampling-integration-test:
	SAMPLING=tail $(MAKE) jaeger-v2-storage-integration-test

# UI reverse-proxy integration tests (UC-1, UC-2, UC-3 from ADR-009).
# Builds a local Docker image from the current source unless JAEGER_IMAGE is set.
.PHONY: ui-reverse-proxy-integration-test
ui-reverse-proxy-integration-test:
	bash ./scripts/e2e/ui-reverse-proxy.sh
