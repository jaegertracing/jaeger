# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHELL := /bin/bash
JAEGER_IMPORT_PATH = github.com/jaegertracing/jaeger

# PLATFORMS is a list of all supported platforms
PLATFORMS="linux/amd64,linux/arm64,linux/s390x,linux/ppc64le,darwin/amd64,darwin/arm64,windows/amd64"
LINUX_PLATFORMS=$(shell echo "$(PLATFORMS)" | tr ',' '\n' | grep linux | tr '\n' ',' | sed 's/,$$/\n/')

# SRC_ROOT is the top of the source tree.
SRC_ROOT := $(shell git rev-parse --show-toplevel)

ifeq ($(DEBUG_BINARY),)
	DISABLE_OPTIMIZATIONS =
	SUFFIX =
	TARGET = release
else
	DISABLE_OPTIMIZATIONS = -gcflags="all=-N -l"
	SUFFIX = -debug
	TARGET = debug
endif


# All .go files that are not auto-generated and should be auto-formatted and linted.
ALL_SRC = $(shell find . -name '*.go' \
				   -not -name '_*' \
				   -not -name '.*' \
				   -not -name 'mocks*' \
				   -not -name '*.pb.go' \
				   -not -path './vendor/*' \
				   -not -path './idl/*' \
				   -not -path './internal/tools/*' \
				   -not -path './scripts/build/docker/debug/*' \
				   -not -path '*/mocks/*' \
				   -not -path '*/thrift-0.9.2/*' \
				   -type f | \
				sort)

# All .sh or .py or Makefile or .mk files that should be auto-formatted and linted.
SCRIPTS_SRC = $(shell find . \( -name '*.sh' -o -name '*.py' -o -name '*.mk' -o -name 'Makefile*' -o -name 'Dockerfile*' \) \
						-not -path './.git/*' \
						-not -path './vendor/*' \
						-not -path './idl/*' \
						-not -path './jaeger-ui/*' \
						-type f | \
					sort)

# ALL_PKGS is used with 'nocover' and 'goleak'
ALL_PKGS = $(shell echo $(dir $(ALL_SRC)) | tr ' ' '\n' | grep -v '/.*-gen/' | sort -u)

GO=go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

# go test does not support -race flag on s390x architecture
ifeq ($(GOARCH), s390x)
	RACE=
else
	RACE=-race
endif
# sed on Mac does not support the same syntax for in-place updates as sed on Linux
# When running on MacOS it's best to install gsed and run Makefile with SED=gsed.
# We want the actual OS here, not what GOOS may have been set to by recursive make calls.
ifeq ($(shell GOOS= $(GO) env GOOS),darwin)
	SED=gsed
else
	SED=sed
endif

GOTEST_QUIET=$(GO) test $(RACE)
GOTEST=$(GOTEST_QUIET) -v
COVEROUT=cover.out
GOFMT=gofmt
FMT_LOG=.fmt.log
IMPORT_LOG=.import.log
COLORIZE ?= | $(SED) 's/PASS/✅ PASS/g' | $(SED) 's/FAIL/❌ FAIL/g' | $(SED) 's/SKIP/🔕 SKIP/g'

 # import other Makefiles after the variables are defined

include scripts/makefiles/BuildBinaries.mk
include scripts/makefiles/BuildInfo.mk
include scripts/makefiles/Docker.mk
include scripts/makefiles/IntegrationTests.mk
include scripts/makefiles/Protobuf.mk
include scripts/makefiles/Tools.mk
include scripts/makefiles/Windows.mk


.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: echo-version
echo-version:
	@echo "$(GIT_CLOSEST_TAG)"

.PHONY: echo-platforms
echo-platforms:
	@echo "$(PLATFORMS)"

.PHONY: echo-linux-platforms
echo-linux-platforms:
	@echo "$(LINUX_PLATFORMS)"

.PHONY: echo-all-pkgs
echo-all-pkgs:
	@echo $(ALL_PKGS) | tr ' ' '\n' | sort

.PHONY: echo-all-srcs
echo-all-srcs:
	@echo $(ALL_SRC) | tr ' ' '\n' | sort

.PHONY: clean
clean:
	rm -rf cover*.out .cover/ cover.html $(FMT_LOG) $(IMPORT_LOG) \
		jaeger-ui/packages/jaeger-ui/build
	find ./cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual -type f -name '*.gz' -delete
	GOCACHE=$(GOCACHE) go clean -cache -testcache
	bash scripts/build/clean-binaries.sh

.PHONY: test
test:
	bash -c "set -e; set -o pipefail; $(GOTEST) -tags=memory_storage_integration ./... $(COLORIZE)"

.PHONY: cover
cover: nocover
	bash -c "set -e; set -o pipefail; STORAGE=memory $(GOTEST) -timeout 5m -coverprofile $(COVEROUT) ./... | tee test-results.json"
	go tool cover -html=cover.out -o cover.html

.PHONY: nocover
nocover:
	@echo Verifying that all packages have test files to count in coverage
	@scripts/lint/check-test-files.sh $(ALL_PKGS)

.PHONY: fmt
fmt: $(GOFUMPT)
	@echo Running import-order-cleanup on ALL_SRC ...
	@./scripts/lint/import-order-cleanup.py -o inplace -t $(ALL_SRC)
	@echo Running gofmt on ALL_SRC ...
	@$(GOFMT) -e -s -l -w $(ALL_SRC)
	@echo Running gofumpt on ALL_SRC ...
	@$(GOFUMPT) -e -l -w $(ALL_SRC)
	@echo Running updateLicense.py on ALL_SRC ...
	@./scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC)

.PHONY: lint
lint: lint-fmt lint-license lint-imports lint-semconv lint-goversion lint-goleak lint-go

.PHONY: lint-license
lint-license:
	@echo Verifying that all files have license headers
	@./scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC) > $(FMT_LOG)
	@[ ! -s "$(FMT_LOG)" ] || (echo "License check failures, run 'make fmt'" | cat - $(FMT_LOG) && false)

.PHONY: lint-nocommit
lint-nocommit:
	@if git diff origin/main | grep '@no''commit' ; then \
		echo "❌ Cannot merge PR that contains @no""commit string" ; \
		GIT_PAGER=cat git diff -G '@no''commit' origin/main ; \
		false ; \
	else \
		echo "✅ Changes do not contain @no""commit string" ; \
	fi

.PHONY: lint-imports
lint-imports:
	@echo Verifying that all Go files have correctly ordered imports
	@./scripts/lint/import-order-cleanup.py -o stdout -t $(ALL_SRC) > $(IMPORT_LOG)
	@[ ! -s "$(IMPORT_LOG)" ] || (echo "Import ordering failures, run 'make fmt'" | cat - $(IMPORT_LOG) && false)

.PHONY: lint-fmt
lint-fmt: $(GOFUMPT)
	@echo Verifying that all Go files are formatted with gofmt and gofumpt
	@rm -f $(FMT_LOG)
	@$(GOFMT) -d -e -s $(ALL_SRC) > $(FMT_LOG) || true
	@$(GOFUMPT) -d -e $(ALL_SRC) >> $(FMT_LOG) || true
	@[ ! -s "$(FMT_LOG)" ] || (echo "Formatting check failed. Please run 'make fmt'" && head -100 $(FMT_LOG) && false)

.PHONY: lint-semconv
lint-semconv:
	./scripts/lint/check-semconv-version.sh

.PHONY: lint-goversion
lint-goversion:
	./scripts/lint/check-go-version.sh

.PHONY: lint-goleak
lint-goleak:
	@echo Verifying that all packages with tests have goleak in their TestMain
	@scripts/lint/check-goleak-files.sh $(ALL_PKGS)

.PHONY: lint-go
lint-go: $(LINT)
	$(LINT) -v run

.PHONY: lint-jaeger-idl-versions
lint-jaeger-idl-versions:
	@echo "checking jaeger-idl version mismatch between git submodule and go.mod dependency"
	@./scripts/lint/check-jaeger-idl-version.sh

.PHONY: run-all-in-one
run-all-in-one: build-ui
	go run ./cmd/all-in-one --log-level debug

.PHONY: changelog
changelog:
	@./scripts/release/notes.py --exclude-dependabot --verbose

.PHONY: draft-release
draft-release:
	./scripts/release/draft.py

.PHONY: prepare-release
prepare-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make prepare-release VERSION=x.x.x"; \
		echo "Example: make prepare-release VERSION=2.14.0"; \
		exit 1; \
	fi
	bash ./scripts/release/prepare.sh $(VERSION)

.PHONY: test-ci
test-ci: GOTEST := $(GOTEST_QUIET)
test-ci: build-examples cover

.PHONY: init-submodules
init-submodules:
	git submodule update --init --recursive

MOCKERY_FLAGS := --all --disable-version-string
.PHONY: generate-mocks
generate-mocks: $(MOCKERY)
	find . -path '*/mocks/*' -name '*.go' -type f -delete
	$(MOCKERY) | tee .mockery.log

.PHONY: certs
certs:
	cd internal/config/tlscfg/testdata && ./gen-certs.sh

.PHONY: certs-dryrun
certs-dryrun:
	cd internal/config/tlscfg/testdata && ./gen-certs.sh -d

.PHONY: repro-check
repro-check:
	# Check local reproducibility of generated executables.
	$(MAKE) clean
	$(MAKE) build-all-platforms
	# Generate checksum for all executables under ./cmd
	find cmd -type f -executable -exec shasum -b -a 256 {} \; | sort -k2 | tee sha256sum.combined.txt
	$(MAKE) clean
	$(MAKE) build-all-platforms
	shasum -b -a 256 --strict --check ./sha256sum.combined.txt



# Test with log capture - runs tests once and captures output
# Fails if tests fail (no masking with || true)
.PHONY: test-with-log
test-with-log:
	@echo "Running tests and capturing logs..."
	@$(MAKE) test 2>&1 | tee test.log; \
	EXIT_CODE=$${PIPESTATUS[0]}; \
	if [ $$EXIT_CODE -ne 0 ]; then \
		echo "❌ Tests failed. See test.log for details."; \
		exit $$EXIT_CODE; \
	fi; \
	echo "✅ Tests passed."

# Verify PR with proof for AI policy compliance
# This target runs lint and tests, uploads logs to a Gist, and adds trailers to the commit.
# Required for new contributors to prove tests were actually run locally.
#
# Prerequisites:
#   - GitHub CLI (gh) installed and authenticated: https://cli.github.com/
#   - At least one commit on your branch
.PHONY: verify-with-proof
verify-with-proof: lint test-with-log
	@# Check that gh CLI is available
	@if ! command -v gh >/dev/null 2>&1; then \
		echo "❌ GitHub CLI (gh) not found. Install from: https://cli.github.com/"; \
		exit 1; \
	fi
	@# Check that there's a commit to amend
	@if ! git rev-parse --verify HEAD >/dev/null 2>&1; then \
		echo "❌ No commit to amend. Please commit your changes first, then run 'make verify-with-proof'."; \
		exit 1; \
	fi
	@echo "✅ Lint and tests passed. Uploading proof to Gist..."
	@# Use tree SHA - it represents the code content and doesn't change when amending commit message
	@TREE_SHA=$$(git rev-parse HEAD^{tree}); \
	echo "Tree SHA: $$TREE_SHA" > test.log.tmp; \
	echo "---" >> test.log.tmp; \
	cat test.log >> test.log.tmp; \
	mv test.log.tmp test.log; \
	GIST_URL=$$(gh gist create test.log -d "Test logs for Jaeger tree $$TREE_SHA" --public); \
	if [ -z "$$GIST_URL" ]; then \
		echo "❌ Failed to create Gist. Make sure 'gh' CLI is authenticated."; \
		echo "   test.log kept for manual inspection."; \
		exit 1; \
	fi; \
	NAME=$$(git config user.name); \
	EMAIL=$$(git config user.email); \
	git commit --amend --no-edit \
		--trailer "Tested-By: $$NAME <$$EMAIL>" \
		--trailer "Test-Gist: $$GIST_URL"; \
	rm -f test.log; \
	echo "✅ Commit amended with Tested-By and Test-Gist trailers."; \
	echo "   Gist URL: $$GIST_URL"; \
	echo ""; \
	echo "Now run: git push --force-with-lease"
