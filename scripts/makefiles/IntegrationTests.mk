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
	$(GOTESTSUM) $(INTEGRATION_TEST_FLAGS) -- $(RACE) -coverpkg=./... -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS)

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
