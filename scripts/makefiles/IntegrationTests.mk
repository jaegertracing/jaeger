# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

STORAGE_PKGS = ./internal/storage/integration/...
JAEGER_V2_STORAGE_PKGS = ./cmd/jaeger/internal/integration
INTEGRATION_TEST_FLAGS = --format standard-verbose --format-icons hivis

.PHONY: all-in-one-integration-test
all-in-one-integration-test: $(GOTESTSUM)
	TEST_MODE=integration $(GOTESTSUM) $(GOTESTSUM_FLAGS) -- $(RACE) ./cmd/jaeger/internal/all_in_one_test.go

# A general integration tests for jaeger-v2 storage backends,
# these tests placed at `./cmd/jaeger/internal/integration/*_test.go`.
# The integration tests are filtered by STORAGE env.
.PHONY: jaeger-v2-storage-integration-test
jaeger-v2-storage-integration-test: $(GOTESTSUM)
	(cd cmd/jaeger/ && go build .)
	# Expire tests results for jaeger storage integration tests since the environment
	# might have changed even though the code remains the same.
	go clean -testcache
	$(GOTESTSUM) $(INTEGRATION_TEST_FLAGS) -- $(RACE) $(RUN_FLAGS) -coverpkg=./... -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS)

# Directory where the @main jaeger binary is installed by install-jaeger-at-main.
# Must match mainJaegerBinary in cmd/jaeger/internal/integration/badger_test.go.
JAEGER_MAIN_INSTALL_DIR = /tmp/jaeger-at-main

# Installs the @main jaeger binary into JAEGER_MAIN_INSTALL_DIR.
# Reusable by other backward-compatibility targets (e.g. for future backends).
.PHONY: install-jaeger-at-main
install-jaeger-at-main:
	mkdir -p $(JAEGER_MAIN_INSTALL_DIR)
	GOBIN=$(JAEGER_MAIN_INSTALL_DIR) go install github.com/jaegertracing/jaeger/cmd/jaeger@main

# Backward compatibility test for jaeger-v2 storage: writes traces with the @main
# binary, then reads them back with the current-branch binary, simulating a rolling
# upgrade against a shared Badger store. The build-and-run logic is reused from
# jaeger-v2-storage-integration-test; only the @main binary provisioning, the
# BACKWARD_COMPATIBILITY toggle, and the test filter are layered on top. The filter
# keeps the env scoped to the backward-compat test so it can't affect the normal suite.
.PHONY: jaeger-v2-backward-compatibility-test
jaeger-v2-backward-compatibility-test: install-jaeger-at-main
	BACKWARD_COMPATIBILITY=true \
	STORAGE=badger \
	$(MAKE) jaeger-v2-storage-integration-test RUN_FLAGS="-run TestBadgerBackwardCompatibility"

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
