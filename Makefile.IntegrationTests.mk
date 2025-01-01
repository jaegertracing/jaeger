# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

STORAGE_PKGS = ./plugin/storage/integration/...
JAEGER_V2_STORAGE_PKGS = ./cmd/jaeger/internal/integration

.PHONY: all-in-one-integration-test
all-in-one-integration-test:
	TEST_MODE=integration $(GOTEST) ./cmd/all-in-one/

# A general integration tests for jaeger-v2 storage backends,
# these tests placed at `./cmd/jaeger/internal/integration/*_test.go`.
# The integration tests are filtered by STORAGE env.
.PHONY: jaeger-v2-storage-integration-test
jaeger-v2-storage-integration-test:
	(cd cmd/jaeger/ && go build .)
	# Expire tests results for jaeger storage integration tests since the environment
	# might have changed even though the code remains the same.
	go clean -testcache
	bash -c "set -e; set -o pipefail; $(GOTEST) -coverpkg=./... -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS) $(COLORIZE)"

.PHONY: storage-integration-test
storage-integration-test:
	# This is for rollover and cleaner tests for elastic search, once the cleaner is also embedded into the jaeger binary, we can shift
    # these tests to v2 integration tests and remove this building of binary here!
	(cd cmd/jaeger/ && go build .)
	# Expire tests results for storage integration tests since the environment might change
	# even though the code remains the same.
	go clean -testcache
	bash -c "set -e; set -o pipefail; $(GOTEST) -coverpkg=./... -coverprofile $(COVEROUT) $(STORAGE_PKGS) $(COLORIZE)"

.PHONY: badger-storage-integration-test
badger-storage-integration-test:
	STORAGE=badger $(MAKE) storage-integration-test

.PHONY: grpc-storage-integration-test
grpc-storage-integration-test:
	STORAGE=grpc $(MAKE) storage-integration-test

# this test assumes STORAGE environment variable is set to elasticsearch|opensearch
.PHONY: index-cleaner-integration-test
index-cleaner-integration-test: docker-images-elastic
	$(MAKE) storage-integration-test COVEROUT=cover-index-cleaner.out

# this test assumes STORAGE environment variable is set to elasticsearch|opensearch
.PHONY: index-rollover-integration-test
index-rollover-integration-test:
	$(MAKE) storage-integration-test COVEROUT=cover-index-rollover.out

.PHONY: tail-sampling-integration-test
tail-sampling-integration-test:
	SAMPLING=tail $(MAKE) jaeger-v2-storage-integration-test
