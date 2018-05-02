PROJECT_ROOT=github.com/jaegertracing/jaeger
TOP_PKGS := $(shell glide novendor | grep -v -e ./thrift-gen/... -e swagger-gen... -e ./examples/... -e ./scripts/...)
STORAGE_PKGS = ./plugin/storage/integration/...

# all .go files that don't exist in hidden directories
ALL_SRC := $(shell find . -name "*.go" | grep -v -e vendor -e thrift-gen -e swagger-gen -e examples -e doc.go \
        -e ".*/\..*" \
        -e ".*/_.*" \
        -e ".*/mocks.*")

ALL_PKGS := $(shell go list $(sort $(dir $(ALL_SRC))))

export GO15VENDOREXPERIMENT=1

RACE=-race
GOTEST=go test -v $(RACE)
GOLINT=golint
GOVET=go vet
GOFMT=gofmt
FMT_LOG=fmt.log
LINT_LOG=lint.log
IMPORT_LOG=import.log

GIT_SHA=$(shell git rev-parse HEAD)
GIT_CLOSEST_TAG=$(shell git describe --abbrev=0 --tags)
DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILD_INFO_IMPORT_PATH=github.com/jaegertracing/jaeger/pkg/version
BUILD_INFO=-ldflags "-X $(BUILD_INFO_IMPORT_PATH).commitSHA=$(GIT_SHA) -X $(BUILD_INFO_IMPORT_PATH).latestVersion=$(GIT_CLOSEST_TAG) -X $(BUILD_INFO_IMPORT_PATH).date=$(DATE)"

SED=sed
THRIFT_VER=0.9.3
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run --rm -u ${shell id -u} -v "${PWD}:/data" $(THRIFT_IMG) thrift
THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_GEN=$(shell which thrift-gen)
THRIFT_GEN_DIR=thrift-gen

SWAGGER_VER=0.12.0
SWAGGER_IMAGE=quay.io/goswagger/swagger:$(SWAGGER_VER)
SWAGGER=docker run --rm -it -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(SWAGGER_IMAGE)
SWAGGER_GEN_DIR=swagger-gen

PASS=$(shell printf "\033[32mPASS\033[0m")
FAIL=$(shell printf "\033[31mFAIL\033[0m")
FIXME=$(shell printf "\033[31mFIXME\033[0m")
COLORIZE=$(SED) ''/PASS/s//$(PASS)/'' | $(SED) ''/FAIL/s//$(FAIL)/''
DOCKER_NAMESPACE?=jaegertracing
DOCKER_TAG?=latest

MOCKERY=mockery

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: go-gen
go-gen:
	go generate $(TOP_PKGS)

.PHONY: md-to-godoc-gen
md-to-godoc-gen:
	find . -name README.md -not -path "./vendor/*" -not -path "./_site/*" -not -path "./idl/*" \
		| grep -v '^./README.md' \
		| xargs -I% md-to-godoc -license -licenseFile LICENSE -input=%

.PHONY: clean
clean:
	rm -rf cover.out cover.html lint.log fmt.log jaeger-ui-build

.PHONY: test
test: go-gen
	bash -c "set -e; set -o pipefail; $(GOTEST) $(TOP_PKGS) | $(COLORIZE)"

.PHONY: integration-test
integration-test: go-gen
	$(GOTEST) -tags=integration ./cmd/standalone/...

.PHONY: storage-integration-test
storage-integration-test: go-gen
	bash -c "set -e; set -o pipefail; $(GOTEST) $(STORAGE_PKGS) | $(COLORIZE)"

all-pkgs:
	@echo $(ALL_PKGS) | tr ' ' '\n' | sort

cvr-pkgs:
	go list $(TOP_PKGS)

.PHONY: cover
cover: nocover
	@echo pre-compiling tests
	@time go test -i $(ALL_PKGS)
	@./scripts/cover.sh $(shell go list $(TOP_PKGS))
	go tool cover -html=cover.out -o cover.html

.PHONY: nocover
nocover:
	@echo Verifying that all packages have test files to count in coverage
	@scripts/check-test-files.sh $(subst github.com/jaegertracing/jaeger/,./,$(ALL_PKGS)) | $(SED) ''/FIXME/s//$(FIXME)/''

.PHONY: fmt
fmt:
	./scripts/import-order-cleanup.sh inplace
	$(GOFMT) -e -s -l -w $(ALL_SRC)
	./scripts/updateLicenses.sh

.PHONY: lint
lint:
	$(GOVET) $(TOP_PKGS)
	@cat /dev/null > $(LINT_LOG)
	@$(foreach pkg, $(TOP_PKGS), $(GOLINT) $(pkg) | grep -v -e pkg/es/wrapper.go -e /mocks/ -e thrift-gen -e thrift-0.9.2 >> $(LINT_LOG) || true;)
	@[ ! -s "$(LINT_LOG)" ] || (echo "Lint Failures" | cat - $(LINT_LOG) && false)
	@$(GOFMT) -e -s -l $(ALL_SRC) > $(FMT_LOG)
	@./scripts/updateLicenses.sh >> $(FMT_LOG)
	@./scripts/import-order-cleanup.sh stdout > $(IMPORT_LOG)
	@[ ! -s "$(FMT_LOG)" -a ! -s "$(IMPORT_LOG)" ] || (echo "Go fmt, license check, or import ordering failures, run 'make fmt'" | cat - $(FMT_LOG) && false)

.PHONY: install-glide
install-glide:
	@which glide > /dev/null || go get github.com/Masterminds/glide

.PHONY: install
install: install-glide
	glide install

.PHONY: install-go-bindata
install-go-bindata:
	go get github.com/jteeuwen/go-bindata/...
	go get github.com/elazarl/go-bindata-assetfs/...

.PHONY: build-examples
build-examples: install-go-bindata
	(cd ./examples/hotrod/services/frontend/ && go-bindata-assetfs -pkg frontend web_assets/...)
	rm ./examples/hotrod/services/frontend/bindata.go
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./examples/hotrod/hotrod-linux ./examples/hotrod/main.go

.PHONE: docker-hotrod
docker-hotrod: build-examples
	docker build -t $(DOCKER_NAMESPACE)/example-hotrod:${DOCKER_TAG} ./examples/hotrod

.PHONY: build_ui
build_ui:
	cd jaeger-ui && yarn install && npm run build
	rm -rf jaeger-ui-build && mkdir jaeger-ui-build
	cp -r jaeger-ui/packages/jaeger-ui/build jaeger-ui-build/

.PHONY: build-all-in-one-linux
build-all-in-one-linux: build_ui
	GOOS=linux $(MAKE) build-all-in-one

.PHONY: build-all-in-one
build-all-in-one:
	CGO_ENABLED=0 installsuffix=cgo go build -o ./cmd/standalone/standalone-$(GOOS) $(BUILD_INFO) ./cmd/standalone/main.go

.PHONY: build-agent
build-agent:
	CGO_ENABLED=0 installsuffix=cgo go build -o ./cmd/agent/agent-$(GOOS) $(BUILD_INFO) ./cmd/agent/main.go

.PHONY: build-query
build-query:
	CGO_ENABLED=0 installsuffix=cgo go build -o ./cmd/query/query-$(GOOS) $(BUILD_INFO) ./cmd/query/main.go

.PHONY: build-collector
build-collector:
	CGO_ENABLED=0 installsuffix=cgo go build -o ./cmd/collector/collector-$(GOOS) $(BUILD_INFO) ./cmd/collector/main.go

.PHONY: docker-no-ui
docker-no-ui: build-binaries-linux build-crossdock-linux
	mkdir -p jaeger-ui-build/build/
	make docker-images-only

.PHONY: docker
docker: build_ui docker-no-ui

.PHONY: build-binaries-linux
build-binaries-linux:
	GOOS=linux $(MAKE) build-agent build-collector build-query build-all-in-one

.PHONY: build-binaries-windows
build-binaries-windows:
	GOOS=windows $(MAKE) build-agent build-collector build-query build-all-in-one

.PHONY: build-binaries-darwin
build-binaries-darwin:
	GOOS=darwin $(MAKE) build-agent build-collector build-query build-all-in-one

.PHONY: docker-images-only
docker-images-only:
	cp -r jaeger-ui-build/build/ cmd/query/jaeger-ui-build
	docker build -t $(DOCKER_NAMESPACE)/jaeger-cassandra-schema:${DOCKER_TAG} plugin/storage/cassandra/
	@echo "Finished building jaeger-cassandra-schema =============="
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-index-cleaner:${DOCKER_TAG} plugin/storage/es
	@echo "Finished building jaeger-es-indices-clean =============="
	for component in agent collector query ; do \
		docker build -t $(DOCKER_NAMESPACE)/jaeger-$$component:${DOCKER_TAG} cmd/$$component ; \
		echo "Finished building $$component ==============" ; \
	done
	rm -rf cmd/query/jaeger-ui-build
	docker build -t $(DOCKER_NAMESPACE)/test-driver:${DOCKER_TAG} crossdock/
	@echo "Finished building test-driver ==============" ; \

.PHONY: docker-push
docker-push:
	@while [ -z "$$CONFIRM" ]; do \
		read -r -p "Do you really want to push images to repository \"${DOCKER_NAMESPACE}\"? [y/N] " CONFIRM; \
	done ; \
	if [ $$CONFIRM != "y" ] && [ $$CONFIRM != "Y" ]; then \
		echo "Exiting." ; exit 1 ; \
	fi
	for component in agent cassandra-schema es-index-cleaner collector query example-hotrod; do \
		docker push $(DOCKER_NAMESPACE)/jaeger-$$component ; \
	done

.PHONY: build-crossdock-linux
build-crossdock-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./crossdock/crossdock-linux ./crossdock/main.go

include crossdock/rules.mk

.PHONY: build-crossdock
build-crossdock: docker-no-ui
	make crossdock

.PHONY: build-crossdock-fresh
build-crossdock-fresh: build-crossdock-linux
	make crossdock-fresh

.PHONY: install-ci
install-ci: install
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover
	go get github.com/golang/lint/golint
	go get github.com/sectioneight/md-to-godoc

.PHONY: test-ci
test-ci: build-examples lint cover

# TODO at the moment we're not generating tchan_*.go files
.PHONY: thrift
thrift: idl/thrift/jaeger.thrift thrift-image
	[ -d $(THRIFT_GEN_DIR) ] || mkdir $(THRIFT_GEN_DIR)
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/agent.thrift
#	TODO sed is GNU and BSD compatible
	sed -i.bak 's|"zipkincore"|"$(PROJECT_ROOT)/thrift-gen/zipkincore"|g' $(THRIFT_GEN_DIR)/agent/*.go
	sed -i.bak 's|"jaeger"|"$(PROJECT_ROOT)/thrift-gen/jaeger"|g' $(THRIFT_GEN_DIR)/agent/*.go
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/jaeger.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/sampling.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/baggage.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/zipkincore.thrift
	@echo Generate TChannel-Thrift bindings
	$(THRIFT_GEN) --inputFile idl/thrift/jaeger.thrift --outputDir $(THRIFT_GEN_DIR)
	$(THRIFT_GEN) --inputFile idl/thrift/sampling.thrift --outputDir $(THRIFT_GEN_DIR)
	$(THRIFT_GEN) --inputFile idl/thrift/baggage.thrift --outputDir $(THRIFT_GEN_DIR)
	$(THRIFT_GEN) --inputFile idl/thrift/zipkincore.thrift --outputDir $(THRIFT_GEN_DIR)
	rm -rf thrift-gen/*/*-remote thrift-gen/*/*.bak

idl/thrift/jaeger.thrift:
	$(MAKE) idl-submodule

.PHONY: idl-submodule
idl-submodule:
	git submodule init
	git submodule update

.PHONY: thrift-image
thrift-image:
	$(THRIFT) -version

.PHONY: generate-zipkin-swagger
generate-zipkin-swagger: idl-submodule
	$(SWAGGER) generate server -f ./idl/swagger/zipkin2-api.yaml -t $(SWAGGER_GEN_DIR) -O PostSpans --exclude-main
	rm $(SWAGGER_GEN_DIR)/restapi/operations/post_spans_urlbuilder.go $(SWAGGER_GEN_DIR)/restapi/server.go $(SWAGGER_GEN_DIR)/restapi/configure_zipkin.go $(SWAGGER_GEN_DIR)/models/trace.go $(SWAGGER_GEN_DIR)/models/list_of_traces.go $(SWAGGER_GEN_DIR)/models/dependency_link.go

.PHONY: install-mockery
install-mockery:
	go get github.com/vektra/mockery

.PHONY: generate-mocks
generate-mocks: install-mockery
	$(MOCKERY) -all -dir ./pkg/es/ -output ./pkg/es/mocks && rm pkg/es/mocks/ClientBuilder.go

.PHONY: echo-version
echo-version:
	@echo $(GIT_CLOSEST_TAG)
