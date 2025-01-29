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
				   -not -path './docker/debug/*' \
				   -not -path '*/mocks/*' \
				   -not -path '*/*-gen/*' \
				   -not -path '*/thrift-0.9.2/*' \
				   -type f | \
				sort)

# All .sh or .py or Makefile or .mk files that should be auto-formatted and linted.
SCRIPTS_SRC = $(shell find . \( -name '*.sh' -o -name '*.py' -o -name '*.mk' -o -name 'Makefile*' -o -name 'Dockerfile*' \) \
						-not -path './.git/*' \
						-not -path './idl/*' \
						-not -path './jaeger-ui/*' \
						-type f | \
					sort)

# ALL_PKGS is used with 'nocover' and 'goleak'
ALL_PKGS = $(shell echo $(dir $(ALL_SRC)) | tr ' ' '\n' | sort -u)

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
# When running on MacOS it's best to install gsed and run Makefile with SED=gsed
ifeq ($(GOOS),darwin)
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
include Makefile.BuildBinaries.mk
include Makefile.BuildInfo.mk
include Makefile.Crossdock.mk
include Makefile.Docker.mk
include Makefile.IntegrationTests.mk
include Makefile.Protobuf.mk
include Makefile.Tools.mk
include Makefile.Windows.mk

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: echo-v1
echo-v1:
	@echo "$(GIT_CLOSEST_TAG_V1)"

.PHONY: echo-v2
echo-v2:
	@echo "$(GIT_CLOSEST_TAG_V2)"

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
	find ./cmd/query/app/ui/actual -type f -name '*.gz' -delete
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
lint: lint-license lint-imports lint-semconv lint-goversion lint-goleak lint-go

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

.PHONY: run-all-in-one
run-all-in-one: build-ui
	go run ./cmd/all-in-one --log-level debug

.PHONY: changelog
changelog:
	./scripts/release/notes.py --exclude-dependabot --verbose

.PHONY: draft-release
draft-release:
	./scripts/release/draft.py

.PHONY: test-ci
test-ci: GOTEST := $(GOTEST_QUIET)
test-ci: build-examples cover

.PHONY: init-submodules
init-submodules:
	git submodule update --init --recursive

MOCKERY_FLAGS := --all --disable-version-string
.PHONY: generate-mocks
generate-mocks: $(MOCKERY)
	$(MOCKERY)

.PHONY: certs
certs:
	cd pkg/config/tlscfg/testdata && ./gen-certs.sh

.PHONY: certs-dryrun
certs-dryrun:
	cd pkg/config/tlscfg/testdata && ./gen-certs.sh -d

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
