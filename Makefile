PROJECT_ROOT=github.com/jaegertracing/jaeger

# all .go files that don't exist in hidden directories
ALL_SRC := $(shell find . -name "*.go" | grep -v -e vendor -e thrift-gen -e swagger-gen \
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
MKDOCS_VIRTUAL_ENV=.mkdocs-virtual-env

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

DEP_VER=v0.3.2

PASS=$(shell printf "\033[32mPASS\033[0m")
FAIL=$(shell printf "\033[31mFAIL\033[0m")
COLORIZE=$(SED) ''/PASS/s//$(PASS)/'' | $(SED) ''/FAIL/s//$(FAIL)/''
DOCKER_NAMESPACE?=jaegertracing
DOCKER_TAG?=latest

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: go-gen
go-gen:
	go generate $(ALL_PKGS)

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
	bash -c "set -e; set -o pipefail; $(GOTEST) $(ALL_PKGS) | $(COLORIZE)"

.PHONY: integration-test
integration-test: go-gen
	$(GOTEST) -tags=integration ./cmd/standalone/...

.PHONY: storage-integration-test
storage-integration-test: go-gen
	$(GOTEST) ./plugin/storage/integration/...

.PHONY: fmt
fmt:
	$(GOFMT) -e -s -l -w $(ALL_SRC)
	./scripts/updateLicenses.sh

.PHONY: lint
lint:
	$(GOVET) $(ALL_PKGS)
	@cat /dev/null > $(LINT_LOG)
	@$(foreach pkg, $(ALL_PKGS), $(GOLINT) $(pkg) | grep -v -e pkg/es/wrapper.go -e /mocks/ -e thrift-gen -e thrift-0.9.2 >> $(LINT_LOG) || true;)
	@[ ! -s "$(LINT_LOG)" ] || (echo "Lint Failures" | cat - $(LINT_LOG) && false)
	@$(GOFMT) -e -s -l $(ALL_SRC) > $(FMT_LOG)
	@./scripts/updateLicenses.sh >> $(FMT_LOG)
	@[ ! -s "$(FMT_LOG)" ] || (echo "Go fmt or license check failures, run 'make fmt'" | cat - $(FMT_LOG) && false)

.PHONY: install-dep
install-dep:
	# all we want is: dep version || go get github.com/golang/dep/cmd/dep
	# but have to be able to pin a specific version
	@which dep > /dev/null || (curl -L -s -o dep https://github.com/golang/dep/releases/download/$(DEP_VER)/dep-linux-amd64 && chmod +x dep && mv dep /usr/local/bin/)

.PHONY: install
install: install-dep
	dep ensure -v -vendor-only

.PHONY: build-examples
build-examples:
	go build -o ./examples/hotrod/hotrod-demo ./examples/hotrod/main.go

.PHONY: build_ui
build_ui:
	cd jaeger-ui && yarn install && npm run build
	rm -rf jaeger-ui-build && mkdir jaeger-ui-build
	cp -r jaeger-ui/build jaeger-ui-build/

.PHONY: build-all-in-one-linux
build-all-in-one-linux: build_ui
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/standalone/standalone-linux $(BUILD_INFO) ./cmd/standalone/main.go

.PHONY: build-agent-linux
build-agent-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/agent/agent-linux $(BUILD_INFO) ./cmd/agent/main.go

.PHONY: build-query-linux
build-query-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/query/query-linux $(BUILD_INFO) ./cmd/query/main.go

.PHONY: build-collector-linux
build-collector-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/collector/collector-linux $(BUILD_INFO) ./cmd/collector/main.go

.PHONY: docker-no-ui
docker-no-ui: build-agent-linux build-collector-linux build-query-linux build-crossdock-linux
	mkdir -p jaeger-ui-build/build/
	make docker-images-only

.PHONY: docker
docker: build_ui docker-no-ui

.PHONY: docker-images-only
docker-images-only:
	cp -r jaeger-ui-build/build/ cmd/query/jaeger-ui-build
	docker build -t $(DOCKER_NAMESPACE)/jaeger-cassandra-schema:${DOCKER_TAG} plugin/storage/cassandra/
	@echo "Finished building jaeger-cassandra-schema =============="
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
	for component in agent cassandra-schema collector query ; do \
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

.PHONY: cover
cover:
	./scripts/cover.sh $(shell go list $(ALL_PKGS))
	go tool cover -html=cover.out -o cover.html

.PHONY: install-ci
install-ci: install
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover
	go get github.com/golang/lint/golint
	go get github.com/sectioneight/md-to-godoc

.PHONY: test-ci
test-ci: build-examples lint
	@echo pre-compiling tests
	@time go test -i $(ALL_PKGS)
	@./scripts/cover.sh $(shell go list $(ALL_PKGS))

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

.PHONY: docs
docs: $(MKDOCS_VIRTUAL_ENV)
	bash -c 'source $(MKDOCS_VIRTUAL_ENV)/bin/activate; mkdocs serve'

$(MKDOCS_VIRTUAL_ENV):
	virtualenv $(MKDOCS_VIRTUAL_ENV)
	bash -c 'source $(MKDOCS_VIRTUAL_ENV)/bin/activate; pip install -r docs/requirements.txt'
