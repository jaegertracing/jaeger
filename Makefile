PROJECT_ROOT=github.com/uber/jaeger
PACKAGES := $(shell glide novendor | grep -v ./thrift-gen/... | grep -v ./examples/...)

# all .go files that don't exist in hidden directories
ALL_SRC := $(shell find . -name "*.go" | grep -v -e vendor -e thrift-gen \
        -e ".*/\..*" \
        -e ".*/_.*" \
        -e ".*/mocks.*")

export GO15VENDOREXPERIMENT=1

RACE=-race
GOTEST=go test -v $(RACE)
GOLINT=golint
GOVET=go vet
GOFMT=gofmt
FMT_LOG=fmt.log
LINT_LOG=lint.log
MKDOCS_VIRTUAL_ENV=.mkdocs-virtual-env

THRIFT_VER=0.9.3
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run --rm -u ${shell id -u} -v "${PWD}:/data" $(THRIFT_IMG) thrift
THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_GEN=$(shell which thrift-gen)
THRIFT_GEN_DIR=thrift-gen

PASS=$(shell printf "\033[32mPASS\033[0m")
FAIL=$(shell printf "\033[31mFAIL\033[0m")
COLORIZE=sed ''/PASS/s//$(PASS)/'' | sed ''/FAIL/s//$(FAIL)/''
DOCKER_NAMESPACE?=$(USER)
DOCKER_TAG?=latest

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: go-gen
go-gen:
	go generate $(PACKAGES)

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
	bash -c "set -e; set -o pipefail; $(GOTEST) $(PACKAGES) | $(COLORIZE)"

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
	$(GOVET) $(PACKAGES)
	@cat /dev/null > $(LINT_LOG)
	@$(foreach pkg, $(PACKAGES), $(GOLINT) $(pkg) | grep -v -e pkg/es/wrapper.go -e /mocks/ -e thrift-gen -e thrift-0.9.2 >> $(LINT_LOG) || true;)
	@[ ! -s "$(LINT_LOG)" ] || (echo "Lint Failures" | cat - $(LINT_LOG) && false)
	@$(GOFMT) -e -s -l $(ALL_SRC) > $(FMT_LOG)
	@./scripts/updateLicenses.sh >> $(FMT_LOG)
	@[ ! -s "$(FMT_LOG)" ] || (echo "Go Fmt Failures, run 'make fmt'" | cat - $(FMT_LOG) && false)

.PHONY: install-glide
install-glide:
	# all we want is: glide --version || go get github.com/Masterminds/glide
	# but have to pin to 0.12.3 due to https://github.com/Masterminds/glide/issues/745
	@which glide > /dev/null || (mkdir -p $(GOPATH)/src/github.com/Masterminds && cd $(GOPATH)/src/github.com/Masterminds && git clone https://github.com/Masterminds/glide.git && cd glide && git checkout v0.12.3 && go install)

.PHONY: install
install: install-glide
	glide install

.PHONY: build_examples
build_examples:
	go build -o ./examples/hotrod/hotrod-demo ./examples/hotrod/main.go

.PHONY: build_ui
build_ui:
	cd jaeger-ui && yarn install && npm run build
	rm -rf jaeger-ui-build && mkdir jaeger-ui-build
	cp -r jaeger-ui/build jaeger-ui-build/

.PHONY: build-all-in-one-linux
build-all-in-one-linux: build_ui
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/standalone/standalone-linux ./cmd/standalone/main.go

.PHONY: build-agent-linux
build-agent-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/agent/agent-linux ./cmd/agent/main.go

.PHONY: build-query-linux
build-query-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/query/query-linux ./cmd/query/main.go

.PHONY: build-collector-linux
build-collector-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./cmd/collector/collector-linux ./cmd/collector/main.go

.PHONY: docker
docker: build_ui build-agent-linux build-collector-linux build-query-linux docker-images-only

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

.PHONY: docker-push
docker-push:
	for component in agent cassandra-schema collector query ; do \
		docker push $(DOCKER_NAMESPACE)/jaeger-$$component ; \
	done

.PHONY: build-crossdock-linux
build-crossdock-linux:
	CGO_ENABLED=0 GOOS=linux installsuffix=cgo go build -o ./crossdock/crossdock ./crossdock/main.go

.PHONY: build-crossdock-bin
build-crossdock-bin:
	make build-crossdock-linux
	make build-query-linux
	make build-collector-linux
	make build-agent-linux

include crossdock/rules.mk

.PHONY: build-crossdock
build-crossdock: build-crossdock-bin
	make crossdock

.PHONY: build-crossdock-fresh
build-crossdock-fresh: build-crossdock-bin
	make crossdock-fresh

.PHONY: cover
cover:
	./scripts/cover.sh $(shell go list $(PACKAGES))
	go tool cover -html=cover.out -o cover.html

.PHONY: install_ci
install_ci: install
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover
	go get github.com/golang/lint/golint
	go get github.com/sectioneight/md-to-godoc

.PHONY: test_ci
test_ci: build_examples
	@./scripts/cover.sh $(shell go list $(PACKAGES))
	make lint

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

.PHONY: docs
docs: $(MKDOCS_VIRTUAL_ENV)
	bash -c 'source $(MKDOCS_VIRTUAL_ENV)/bin/activate; mkdocs serve'

$(MKDOCS_VIRTUAL_ENV):
	virtualenv $(MKDOCS_VIRTUAL_ENV)
	bash -c 'source $(MKDOCS_VIRTUAL_ENV)/bin/activate; pip install -r docs/requirements.txt'
