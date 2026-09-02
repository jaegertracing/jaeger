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
jaeger-v2-storage-integration-test: $(GOTESTSUM) $(GOCOVMERGE)
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

# Runs only the backward-compatibility suites out of the jaeger-v2 storage tests. Those suites
# write the corpus with a second Jaeger built from an earlier revision, so they need JAEGER_OLD_BINARY
# and JAEGER_OLD_CONFIG_DIR to name that binary and that revision's cmd/jaeger directory. Without
# them each suite skips itself, so the check below is what keeps a workflow that forgot to set them
# from reporting success having tested nothing.
.PHONY: jaeger-v2-backward-compatibility-test
jaeger-v2-backward-compatibility-test:
ifndef JAEGER_OLD_BINARY
	$(error JAEGER_OLD_BINARY must point at a Jaeger binary built from an earlier revision)
endif
ifndef JAEGER_OLD_CONFIG_DIR
	$(error JAEGER_OLD_CONFIG_DIR must point at the cmd/jaeger directory of that earlier revision)
endif
	$(MAKE) jaeger-v2-storage-integration-test EXTRA_TEST_ARGS='-run BackwardCompatibility'

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
