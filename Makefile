# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHELL := $(if $(filter Windows_NT,$(OS)),bash,/bin/bash)
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
LIST_SRC = find . -name '*.go' \
				   -not -name '_*' \
				   -not -name '.*' \
				   -not -name 'mocks*' \
				   -not -name '*.pb.go' \
				   -not -path './.claude/*' \
				   -not -path './vendor/*' \
				   -not -path './idl/*' \
				   -not -path './jaeger-ui/*' \
				   -not -path './internal/tools/*' \
				   -not -path '*/mocks/*' \
				   -not -path './cmd/jaeger/_build/*' \
				   -type f | \
				sort
ALL_SRC = $(shell $(LIST_SRC))

# All tracked file types that should carry copyright and SPDX license headers.
LIST_LICENSE_SRC = git ls-files \
						'*.go' '*.js' '*.mk' '*.py' '*.sh' '*.ts' '*.yaml' '*.yml' 'Dockerfile*' 'Makefile*' | \
						grep -vE '^(vendor/|idl/|jaeger-ui/|internal/proto-gen/|sdk/python/src/(api_v3|expression)/|python-sidecar/\.venv/|.*(^|/)mocks/|.*_mock\.go$$|.*\.pb\.go$$)' | \
						sort

# All .sh or .py or Makefile or .mk files that should be auto-formatted and linted.
SCRIPTS_SRC = $(shell find . \( -name '*.sh' -o -name '*.py' -o -name '*.mk' -o -name 'Makefile*' -o -name 'Dockerfile*' \) \
						-not -path './.git/*' \
						-not -path './.claude/*' \
						-not -path './vendor/*' \
						-not -path './idl/*' \
						-not -path './jaeger-ui/*' \
						-type f | \
					sort)

# ALL_PKGS is used with 'nocover' and 'goleak'
LIST_PKGS = $(LIST_SRC) | sed 's|/[^/]*$$|/|' | grep -v '/.*-gen/' | sort -u
ALL_PKGS = $(shell $(LIST_PKGS))

GO=go
PYTHON ?= $(if $(filter Windows_NT,$(OS)),python,python3)
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

# Every coverage artifact lives under COVERDIR so that nothing is written to the
# repository root. `go test` and `covdata` do not create the directory they write
# into, so each target that produces coverage creates it first.
COVERDIR = .cover
COVEROUT = $(COVERDIR)/cover.out
COVERHTML = $(COVERDIR)/cover.html

# gotestsum reruns a failed package by invoking `go test` again with the same
# flags, and `go test -coverprofile` truncates the file it is given, so a rerun
# leaves a profile holding only the package that was rerun. Writing binary
# counters into a directory instead accumulates them: each invocation adds
# uniquely named files, and covdata merges the whole set afterwards.
UNIT_COVERDIR = $(CURDIR)/$(COVERDIR)/unit
GOFMT=gofmt
FMT_LOG=.fmt.log
IMPORT_LOG=.import.log
GOTESTSUM_FLAGS=--format pkgname-and-test-fails --format-icons hivis

# Import other Makefiles after the variables are defined.
# The order is important as some Makefiles depend on variables
# defined in this file and other includes.

include scripts/makefiles/Tools.mk
include scripts/makefiles/BuildBinaries.mk
include scripts/makefiles/BuildInfo.mk
include scripts/makefiles/Docker.mk
include scripts/makefiles/IntegrationTests.mk
include scripts/makefiles/Protobuf.mk
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
	rm -rf $(COVERDIR)/ $(FMT_LOG) $(IMPORT_LOG) \
		jaeger-ui/packages/jaeger-ui/build
	find ./cmd/jaeger/internal/extension/jaegerquery/internal/ui/actual -type f -name '*.gz' -delete
	GOCACHE=$(GOCACHE) go clean -cache -testcache
	bash scripts/build/clean-binaries.sh

.PHONY: test
test: $(GOTESTSUM)
	$(GOTESTSUM) $(GOTESTSUM_FLAGS) -- $(RACE) -tags=memory_storage_integration ./...

.PHONY: cover
cover: nocover $(GOTESTSUM)
	rm -rf $(UNIT_COVERDIR) && mkdir -p $(UNIT_COVERDIR)
	# -covermode=atomic is pinned rather than left to -race, because gocovmerge
	# refuses to merge this profile with the e2e legs' profiles unless the modes
	# match, and the CI Summary Report merges all of them.
	STORAGE=memory $(GOTESTSUM) $(GOTESTSUM_FLAGS) --rerun-fails --packages ./... -- $(RACE) -timeout 5m -cover -covermode=atomic -args -test.gocoverdir=$(UNIT_COVERDIR)
	go tool covdata textfmt -i=$(UNIT_COVERDIR) -o=$(COVEROUT)
	go tool cover -html=$(COVEROUT) -o $(COVERHTML)

.PHONY: nocover
nocover:
	@echo Verifying that all packages have test files to count in coverage
	@set -o pipefail && $(LIST_PKGS) | xargs -n 50 bash scripts/lint/check-test-files.sh

.PHONY: fmt
fmt: $(GOFUMPT)
	@echo Running import-order-cleanup on ALL_SRC ...
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(PYTHON) scripts/lint/import-order-cleanup.py -o inplace -t
	@echo Running gofmt on ALL_SRC ...
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(GOFMT) -e -s -l -w
	@echo Running gofumpt on ALL_SRC ...
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(GOFUMPT) -e -l -w
	@echo Running updateLicense.py on LICENSE_SRC ...
	@set -o pipefail && $(LIST_LICENSE_SRC) | xargs -n 50 $(PYTHON) scripts/lint/updateLicense.py
	@echo Running check-line-endings on all files ...
	@$(PYTHON) scripts/lint/check-line-endings.py -u

.PHONY: lint
lint: lint-fmt lint-license lint-imports lint-semconv lint-goversion lint-goleak lint-go lint-monitoring lint-line-endings

.PHONY: lint-monitoring
lint-monitoring:
	@cd ./monitoring/jaeger-mixin/generate && go run . | diff -q ../dashboard-for-grafana.json - > /dev/null || \
		(echo "ERROR: dashboard-for-grafana.json is out of sync. Run 'make generate-dashboards'."; exit 1)
	@echo "OK: dashboard-for-grafana.json is in sync."

.PHONY: lint-license
lint-license:
	@echo Verifying that all files have license headers
	@set -o pipefail && $(LIST_LICENSE_SRC) | xargs -n 50 $(PYTHON) scripts/lint/updateLicense.py > $(FMT_LOG)
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
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(PYTHON) scripts/lint/import-order-cleanup.py -o stdout -t > $(IMPORT_LOG)
	@[ ! -s "$(IMPORT_LOG)" ] || (echo "Import ordering failures, run 'make fmt'" | cat - $(IMPORT_LOG) && false)

.PHONY: lint-line-endings
lint-line-endings:
	@echo Verifying that all files use Unix line endings, have no trailing whitespace, and end with a final newline at EOF
	@$(PYTHON) scripts/lint/check-line-endings.py

.PHONY: lint-fmt
lint-fmt: $(GOFUMPT)
	@echo Verifying that all Go files are formatted with gofmt and gofumpt
	@rm -f $(FMT_LOG)
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(GOFMT) -d -e -s > $(FMT_LOG)
	@set -o pipefail && $(LIST_SRC) | xargs -n 50 $(GOFUMPT) -d -e >> $(FMT_LOG)
	@[ ! -s "$(FMT_LOG)" ] || (echo "Formatting check failed. Please run 'make fmt'" && head -100 $(FMT_LOG) && false)

.PHONY: lint-semconv
lint-semconv:
	bash scripts/lint/check-semconv-version.sh

.PHONY: lint-goversion
lint-goversion:
	bash scripts/lint/check-go-version.sh

.PHONY: lint-goleak
lint-goleak:
	@echo Verifying that all packages with tests have goleak in their TestMain
	@set -o pipefail && $(LIST_PKGS) | xargs -n 50 bash scripts/lint/check-goleak-files.sh

# Each checkout gets its own cache because golangci-lint keys entries by file
# content rather than by path, so worktrees sharing one cache report findings
# against each other's paths.
.PHONY: lint-go
lint-go: $(LINT)
	GOLANGCI_LINT_CACHE=$(TOOLS_BIN_DIR)/golangci-lint-cache $(LINT) -v run

.PHONY: govulncheck
govulncheck: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

.PHONY: lint-jaeger-idl-versions
lint-jaeger-idl-versions:
	@echo "checking jaeger-idl version mismatch between git submodule and go.mod dependency"
	@./scripts/lint/check-jaeger-idl-version.sh

.PHONY: run-all-in-one
run-all-in-one: build-ui
	go run ./cmd/all-in-one --log-level debug

.PHONY: run-ai-gemini
run-ai-gemini:
	./scripts/ai-sidecar/gemini/run.sh

.PHONY: changelog
changelog:
	@./scripts/release/notes.py --exclude-dependabot --verbose

.PHONY: draft-release
draft-release:
	./scripts/release/draft.py

.PHONY: prepare-release
prepare-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make prepare-release VERSION=x.x.x [ISSUE=nnnn]"; \
		echo "Example: make prepare-release VERSION=2.14.0 ISSUE=1234"; \
		exit 1; \
	fi
	bash ./scripts/release/prepare.sh $(VERSION) $(ISSUE)

.PHONY: test-ci
test-ci: build-examples cover

.PHONY: init-submodules
init-submodules:
	git submodule update --init --recursive

MOCKERY_FLAGS := --all --disable-version-string
.PHONY: generate-dashboards
generate-dashboards:
	cd ./monitoring/jaeger-mixin/generate && go run . > ../dashboard-for-grafana.json

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
