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
# Coverage of the jaeger binary the e2e harness spawns. The harness runs jaeger
# as a separate OS process, so `go test -coverpkg` in the test process cannot see
# it — the tests only drive it over the wire. Instead the binary itself is built
# with `go build -cover` and writes counters into GOCOVERDIR when it exits, which
# is why Binary.Stop must terminate it with SIGTERM rather than SIGKILL: Go
# flushes counters from the runtime exit path, and SIGKILL skips it.
#
# The counters are converted to a profile and merged into COVEROUT, so Codecov
# and the CI Summary Report fan-in pick them up through the existing upload with
# no workflow changes.
#
# Both sides pin -covermode=atomic because gocovmerge refuses to merge profiles
# with different modes: `go test -race` silently promotes the test profile to
# atomic while covdata emits set, so leaving the mode implicit makes the merge
# succeed locally (no -race) and fail in CI (-race).
BINARY_COVERDIR = $(CURDIR)/.cover-binary
BINARY_COVEROUT = cover-binary.out

.PHONY: jaeger-v2-storage-integration-test
jaeger-v2-storage-integration-test: $(GOTESTSUM) $(GOCOVMERGE) $(PRE_TEST)
	rm -rf $(BINARY_COVERDIR) && mkdir -p $(BINARY_COVERDIR)
	go build -cover -covermode=atomic -o ./cmd/jaeger/jaeger-e2e ./cmd/jaeger/internal/integration/jaeger-e2e
	# Expire tests results for jaeger storage integration tests since the environment
	# might have changed even though the code remains the same.
	go clean -testcache
	JAEGER_BINARY_COVERDIR=$(BINARY_COVERDIR) $(GOTESTSUM) $(INTEGRATION_TEST_FLAGS) -- $(RACE) $(EXTRA_TEST_ARGS) -covermode=atomic -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS)
	# Require both files. The meta file is written when the instrumented binary
	# starts, the counters only when it exits normally, so an abnormal exit leaves
	# meta alone. covdata is happy to convert that: it exits 0 and emits a profile
	# listing every instrumented statement at zero, which merged into COVEROUT would
	# add ~22k uncovered statements and collapse the reported total — surfacing as a
	# coverage regression rather than as the missing counters it actually is.
	# Treat that as "no binary coverage"; the absent contribution is then reported by
	# scripts/e2e/check_coverage_uploads.py.
	@if ls $(BINARY_COVERDIR)/covmeta.* >/dev/null 2>&1 && ls $(BINARY_COVERDIR)/covcounters.* >/dev/null 2>&1; then \
		set -e; \
		go tool covdata textfmt -i=$(BINARY_COVERDIR) -o $(BINARY_COVEROUT); \
		$(GOCOVMERGE) $(COVEROUT) $(BINARY_COVEROUT) > $(COVEROUT).tmp; \
		mv $(COVEROUT).tmp $(COVEROUT); \
		echo "Merged binary coverage into $(COVEROUT)"; \
	else \
		echo "WARNING: no binary coverage counters in $(BINARY_COVERDIR); did the binary exit cleanly?"; \
	fi

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
