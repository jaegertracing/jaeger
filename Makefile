# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHELL := /bin/bash
JAEGER_IMPORT_PATH = github.com/jaegertracing/jaeger
STORAGE_PKGS = ./plugin/storage/integration/...
JAEGER_V2_STORAGE_PKGS = ./cmd/jaeger/internal/integration

# These DOCKER_xxx vars are used when building Docker images.
DOCKER_NAMESPACE?=jaegertracing
DOCKER_TAG?=latest

# SRC_ROOT is the top of the source tree.
SRC_ROOT := $(shell git rev-parse --show-toplevel)

# TODO we can compartmentalize this Makefile better, by separting:
#  - integration tests
#  - all the binary building targets

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
				   -not -path './internal/tools/*' \
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

UNAME := $(shell uname -m)
ifeq ($(UNAME), s390x)
# go test does not support -race flag on s390x architecture
	RACE=
else
	RACE=-race
endif
# sed on Mac does not support the same syntax for in-place updates as sed on Linux
# When running on MacOS it's best to install gsed and run Makefile with SED=gsed
SED=sed
GO=go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
GOBUILD=CGO_ENABLED=0 installsuffix=cgo $(GO) build -trimpath
GOTEST_QUIET=$(GO) test $(RACE)
GOTEST=$(GOTEST_QUIET) -v
COVEROUT=cover.out
GOFMT=gofmt
FMT_LOG=.fmt.log
IMPORT_LOG=.import.log
COLORIZE ?= | $(SED) 's/PASS/✅ PASS/g' | $(SED) 's/FAIL/❌ FAIL/g' | $(SED) 's/SKIP/🔕 SKIP/g'

GIT_SHA=$(shell git rev-parse HEAD)
GIT_SHALLOW_CLONE := $(shell git rev-parse --is-shallow-repository)
# Some of GitHub Actions workflows do a shallow checkout without tags. This avoids logging warnings from git.
GIT_CLOSEST_TAG=$(shell if [ "$(GIT_SHALLOW_CLONE)" = "false" ]; then git describe --abbrev=0 --tags; else echo 0.0.0; fi)
ifneq ($(GIT_CLOSEST_TAG),$(shell echo ${GIT_CLOSEST_TAG} | grep -E "$(semver_regex)"))
	$(warning GIT_CLOSEST_TAG=$(GIT_CLOSEST_TAG) is not in the semver format $(semver_regex))
endif
GIT_CLOSEST_TAG_MAJOR := $(shell echo $(GIT_CLOSEST_TAG) | $(SED) -n 's/v\([0-9]*\)\.[0-9]*\.[0-9]/\1/p')
GIT_CLOSEST_TAG_MINOR := $(shell echo $(GIT_CLOSEST_TAG) | $(SED) -n 's/v[0-9]*\.\([0-9]*\)\.[0-9]/\1/p')
GIT_CLOSEST_TAG_PATCH := $(shell echo $(GIT_CLOSEST_TAG) | $(SED) -n 's/v[0-9]*\.[0-9]*\.\([0-9]\)/\1/p')
DATE=$(shell TZ=UTC0 git show --quiet --date='format-local:%Y-%m-%dT%H:%M:%SZ' --format="%cd")
BUILD_INFO_IMPORT_PATH=$(JAEGER_IMPORT_PATH)/pkg/version
BUILD_INFO=-ldflags "-X $(BUILD_INFO_IMPORT_PATH).commitSHA=$(GIT_SHA) -X $(BUILD_INFO_IMPORT_PATH).latestVersion=$(GIT_CLOSEST_TAG) -X $(BUILD_INFO_IMPORT_PATH).date=$(DATE)"

SYSOFILE=resource.syso

# import other Makefiles after the variables are defined
include Makefile.Tools.mk
include docker/Makefile
include Makefile.Protobuf.mk
include Makefile.Thrift.mk
include Makefile.Crossdock.mk

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: echo-version
echo-version:
	@echo "$(GIT_CLOSEST_TAG)"

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
	find cmd -type f -executable | xargs -I{} sh -c '(git ls-files --error-unmatch {} 2>/dev/null || rm -v {})'

.PHONY: test
test:
	bash -c "set -e; set -o pipefail; $(GOTEST) -tags=memory_storage_integration ./... $(COLORIZE)"

.PHONY: all-in-one-integration-test
all-in-one-integration-test:
	TEST_MODE=integration $(GOTEST) ./cmd/all-in-one/

# A general integration tests for jaeger-v2 storage backends,
# these tests placed at `./cmd/jaeger/internal/integration/*_test.go`.
# The integration tests are filtered by STORAGE env,
# currently the available STORAGE variable is:
#  - grpc
.PHONY: jaeger-v2-storage-integration-test
jaeger-v2-storage-integration-test:
	(cd cmd/jaeger/ && go build .)
	# Expire tests results for jaeger storage integration tests since the environment might change
	# even though the code remains the same.
	go clean -testcache
	bash -c "set -e; set -o pipefail; $(GOTEST) -coverpkg=./... -coverprofile $(COVEROUT) $(JAEGER_V2_STORAGE_PKGS) $(COLORIZE)"

.PHONY: storage-integration-test
storage-integration-test:
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
index-rollover-integration-test: docker-images-elastic
	$(MAKE) storage-integration-test COVEROUT=cover-index-rollover.out

.PHONY: cover
cover: nocover
	bash -c "set -e; set -o pipefail; STORAGE=memory $(GOTEST) -timeout 5m -coverprofile $(COVEROUT) ./... | tee test-results.json"
	go tool cover -html=cover.out -o cover.html

.PHONY: nocover
nocover:
	@echo Verifying that all packages have test files to count in coverage
	@scripts/check-test-files.sh $(ALL_PKGS)

.PHONY: goleak
goleak:
	@echo Verifying that all packages with tests have goleak in their TestMain
	@scripts/check-goleak-files.sh $(ALL_PKGS)

.PHONY: fmt
fmt: $(GOFUMPT)
	@echo Running import-order-cleanup on ALL_SRC ...
	@./scripts/import-order-cleanup.py -o inplace -t $(ALL_SRC)
	@echo Running gofmt on ALL_SRC ...
	@$(GOFMT) -e -s -l -w $(ALL_SRC)
	@echo Running gofumpt on ALL_SRC ...
	@$(GOFUMPT) -e -l -w $(ALL_SRC)
	@echo Running updateLicense.py on ALL_SRC ...
	@./scripts/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC)

.PHONY: lint
lint: $(LINT) goleak
	@./scripts/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC) > $(FMT_LOG)
	@./scripts/import-order-cleanup.py -o stdout -t $(ALL_SRC) > $(IMPORT_LOG)
	@[ ! -s "$(FMT_LOG)" -a ! -s "$(IMPORT_LOG)" ] || (echo "License check or import ordering failures, run 'make fmt'" | cat - $(FMT_LOG) $(IMPORT_LOG) && false)
	./scripts/check-semconv-version.sh
	./scripts/check-go-version.sh
	$(LINT) -v run

.PHONY: build-examples
build-examples:
	$(GOBUILD) -o ./examples/hotrod/hotrod-$(GOOS)-$(GOARCH) ./examples/hotrod/main.go

.PHONY: build-tracegen
build-tracegen:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/tracegen/tracegen-$(GOOS)-$(GOARCH) ./cmd/tracegen/

.PHONY: build-anonymizer
build-anonymizer:
	$(GOBUILD) $(BUILD_INFO) -o ./cmd/anonymizer/anonymizer-$(GOOS)-$(GOARCH) $(BUILD_INFO) ./cmd/anonymizer/

.PHONY: build-esmapping-generator
build-esmapping-generator:
	$(GOBUILD) -o ./plugin/storage/es/esmapping-generator-$(GOOS)-$(GOARCH) $(BUILD_INFO) ./cmd/esmapping-generator/

.PHONY: build-esmapping-generator-linux
build-esmapping-generator-linux:
	 GOOS=linux GOARCH=amd64 $(GOBUILD) -o ./plugin/storage/es/esmapping-generator $(BUILD_INFO) ./cmd/esmapping-generator/

.PHONY: build-es-index-cleaner
build-es-index-cleaner:
	$(GOBUILD) -o ./cmd/es-index-cleaner/es-index-cleaner-$(GOOS)-$(GOARCH) ./cmd/es-index-cleaner/

.PHONY: build-es-rollover
build-es-rollover:
	$(GOBUILD) -o ./cmd/es-rollover/es-rollover-$(GOOS)-$(GOARCH) ./cmd/es-rollover/

.PHONY: docker-hotrod
docker-hotrod:
	GOOS=linux $(MAKE) build-examples
	docker build -t $(DOCKER_NAMESPACE)/example-hotrod:${DOCKER_TAG} ./examples/hotrod --build-arg TARGETARCH=$(GOARCH)

.PHONY: run-all-in-one
run-all-in-one: build-ui
	go run -tags ui ./cmd/all-in-one --log-level debug

build-ui: cmd/query/app/ui/actual/index.html.gz

cmd/query/app/ui/actual/index.html.gz: jaeger-ui/packages/jaeger-ui/build/index.html
	# do not delete dot-files
	rm -rf cmd/query/app/ui/actual/*
	cp -r jaeger-ui/packages/jaeger-ui/build/* cmd/query/app/ui/actual/
	find cmd/query/app/ui/actual -type f | grep -v .gitignore | xargs gzip --no-name
	# copy the timestamp for index.html.gz from the original file
	touch -t $$(date -r jaeger-ui/packages/jaeger-ui/build/index.html '+%Y%m%d%H%M.%S') cmd/query/app/ui/actual/index.html.gz
	ls -lF cmd/query/app/ui/actual/

jaeger-ui/packages/jaeger-ui/build/index.html:
	$(MAKE) rebuild-ui

.PHONY: rebuild-ui
rebuild-ui:
	bash ./scripts/rebuild-ui.sh
	@echo "NOTE: This target only rebuilds the UI assets inside jaeger-ui/packages/jaeger-ui/build/."
	@echo "NOTE: To make them usable from query-service run 'make build-ui'."

.PHONY: build-all-in-one-linux
build-all-in-one-linux:
	GOOS=linux $(MAKE) build-all-in-one

# Requires variables: $(BIN_NAME) $(BIN_PATH) $(GO_TAGS) $(DISABLE_OPTIMIZATIONS) $(SUFFIX) $(GOOS) $(GOARCH) $(BUILD_INFO)
# Other targets can depend on this one but with a unique suffix to ensure it is always executed.
BIN_PATH = ./cmd/$(BIN_NAME)
.PHONY: _build-a-binary
_build-a-binary-%:
	$(GOBUILD) $(DISABLE_OPTIMIZATIONS) $(GO_TAGS) -o $(BIN_PATH)/$(BIN_NAME)$(SUFFIX)-$(GOOS)-$(GOARCH) $(BUILD_INFO) $(BIN_PATH)

.PHONY: build-jaeger
build-jaeger: BIN_NAME = jaeger
build-jaeger: GO_TAGS = -tags ui
build-jaeger: build-ui _build-a-binary-jaeger$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-all-in-one
build-all-in-one: BIN_NAME = all-in-one
build-all-in-one: GO_TAGS = -tags ui
build-all-in-one: build-ui _build-a-binary-all-in-one$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-agent
build-agent: BIN_NAME = agent
build-agent: _build-a-binary-agent$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-query
build-query: BIN_NAME = query
build-query: GO_TAGS = -tags ui
build-query: build-ui _build-a-binary-query$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-collector
build-collector: BIN_NAME = collector
build-collector: _build-a-binary-collector$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-ingester
build-ingester: BIN_NAME = ingester
build-ingester: _build-a-binary-ingester$(SUFFIX)-$(GOOS)-$(GOARCH)

.PHONY: build-remote-storage
build-remote-storage: BIN_NAME = remote-storage
build-remote-storage: _build-a-binary-remote-storage$(SUFFIX)-$(GOOS)-$(GOARCH)

# Magic values:
# - LangID "0409" is "US-English".
# - CharsetID "04B0" translates to decimal 1200 for "Unicode".
# - FileOS "040004" defines the Windows kernel "Windows NT".
# - FileType "01" is "Application".
define VERSIONINFO
{
    "FixedFileInfo": {
        "FileVersion": {
            "Major": $(GIT_CLOSEST_TAG_MAJOR),
            "Minor": $(GIT_CLOSEST_TAG_MINOR),
            "Patch": $(GIT_CLOSEST_TAG_PATCH),
            "Build": 0
        },
        "ProductVersion": {
            "Major": $(GIT_CLOSEST_TAG_MAJOR),
            "Minor": $(GIT_CLOSEST_TAG_MINOR),
            "Patch": $(GIT_CLOSEST_TAG_PATCH),
            "Build": 0
        },
        "FileFlagsMask": "3f",
        "FileFlags ": "00",
        "FileOS": "040004",
        "FileType": "01",
        "FileSubType": "00"
    },
    "StringFileInfo": {
        "FileDescription": "$(NAME)",
        "FileVersion": "$(GIT_CLOSEST_TAG_MAJOR).$(GIT_CLOSEST_TAG_MINOR).$(GIT_CLOSEST_TAG_PATCH).0",
        "LegalCopyright": "2015-2023 The Jaeger Project Authors",
		"ProductName": "$(NAME)",
        "ProductVersion": "$(GIT_CLOSEST_TAG_MAJOR).$(GIT_CLOSEST_TAG_MINOR).$(GIT_CLOSEST_TAG_PATCH).0"
    },
    "VarFileInfo": {
        "Translation": {
            "LangID": "0409",
            "CharsetID": "04B0"
        }
    }
}
endef

export VERSIONINFO

.PHONY: _prepare-winres
_prepare-winres:
	$(MAKE) _prepare-winres-helper NAME="Jaeger Agent"            PKGPATH="cmd/agent"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Collector"        PKGPATH="cmd/collector"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Query"            PKGPATH="cmd/query"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Ingester"         PKGPATH="cmd/ingester"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Remote Storage"   PKGPATH="cmd/remote-storage"
	$(MAKE) _prepare-winres-helper NAME="Jaeger All-In-One"       PKGPATH="cmd/all-in-one"
	$(MAKE) _prepare-winres-helper NAME="Jaeger V2"               PKGPATH="cmd/jaeger"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Tracegen"         PKGPATH="cmd/tracegen"
	$(MAKE) _prepare-winres-helper NAME="Jaeger Anonymizer"       PKGPATH="cmd/anonymizer"
	$(MAKE) _prepare-winres-helper NAME="Jaeger ES-Index-Cleaner" PKGPATH="cmd/es-index-cleaner"
	$(MAKE) _prepare-winres-helper NAME="Jaeger ES-Rollover"      PKGPATH="cmd/es-rollover"

.PHONY: _prepare-winres-helper
_prepare-winres-helper:
	echo $$VERSIONINFO | $(GOVERSIONINFO) -o="$(PKGPATH)/$(SYSOFILE)" -

.PHONY: build-binaries-linux
build-binaries-linux: build-binaries-amd64

.PHONY: build-binaries-amd64
build-binaries-amd64:
	GOOS=linux GOARCH=amd64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-windows
build-binaries-windows: _prepare-winres
	GOOS=windows GOARCH=amd64 $(MAKE) _build-platform-binaries
	rm ./cmd/*/$(SYSOFILE)

.PHONY: build-binaries-darwin
build-binaries-darwin:
	GOOS=darwin GOARCH=amd64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-darwin-arm64
build-binaries-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-s390x
build-binaries-s390x:
	GOOS=linux GOARCH=s390x $(MAKE) _build-platform-binaries

.PHONY: build-binaries-arm64
build-binaries-arm64:
	GOOS=linux GOARCH=arm64 $(MAKE) _build-platform-binaries

.PHONY: build-binaries-ppc64le
build-binaries-ppc64le:
	GOOS=linux GOARCH=ppc64le $(MAKE) _build-platform-binaries

# build all binaries for one specific platform GOOS/GOARCH
.PHONY: _build-platform-binaries
_build-platform-binaries: build-agent \
		build-all-in-one \
		build-collector \
		build-query \
		build-ingester \
		build-jaeger \
		build-remote-storage \
		build-examples \
		build-tracegen \
		build-anonymizer \
		build-esmapping-generator \
		build-es-index-cleaner \
		build-es-rollover
	$(MAKE) _build-platform-binaries-debug GOOS=$(GOOS) GOARCH=$(GOARCH) DEBUG_BINARY=1

# build binaries that support DEBUG release, for one specific platform GOOS/GOARCH
.PHONY: _build-platform-binaries-debug
_build-platform-binaries-debug: build-agent \
	build-collector \
	build-query \
	build-ingester \
	build-remote-storage \
	build-all-in-one \

.PHONY: build-all-platforms
build-all-platforms: \
	build-binaries-linux \
	build-binaries-windows \
	build-binaries-darwin \
	build-binaries-darwin-arm64 \
	build-binaries-s390x \
	build-binaries-arm64 \
	build-binaries-ppc64le

.PHONY: docker-images-cassandra
docker-images-cassandra:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-cassandra-schema:${DOCKER_TAG} plugin/storage/cassandra/
	@echo "Finished building jaeger-cassandra-schema =============="

.PHONY: docker-images-elastic
docker-images-elastic: create-baseimg
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-esmapping-generator
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-es-index-cleaner
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-es-rollover
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-index-cleaner:${DOCKER_TAG} --build-arg base_image=$(BASE_IMAGE) --build-arg TARGETARCH=$(GOARCH) cmd/es-index-cleaner
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-rollover:${DOCKER_TAG} --build-arg base_image=$(BASE_IMAGE) --build-arg TARGETARCH=$(GOARCH) cmd/es-rollover
	@echo "Finished building jaeger-es-indices-clean =============="

.PHONY: docker-images-tracegen
docker-images-tracegen:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-tracegen:${DOCKER_TAG} cmd/tracegen/ --build-arg TARGETARCH=$(GOARCH)
	@echo "Finished building jaeger-tracegen =============="

.PHONY: docker-images-anonymizer
docker-images-anonymizer:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-anonymizer:${DOCKER_TAG} cmd/anonymizer/ --build-arg TARGETARCH=$(GOARCH)
	@echo "Finished building jaeger-anonymizer =============="

.PHONY: changelog
changelog:
	./scripts/release-notes.py --exclude-dependabot --verbose

.PHONY: draft-release
draft-release:
	./scripts/draft-release.py

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
