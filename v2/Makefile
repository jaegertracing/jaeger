VERSION ?= 2.0.0-apha2

OTELCOL_BUILDER_VERSION ?= 0.22.0
OTELCOL_VERSION ?= 0.22.0
OTELCOL_BUILDER_DIR ?= ~/bin
OTELCOL_BUILDER ?= $(OTELCOL_BUILDER_DIR)/opentelemetry-collector-builder

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# If we are running in CI, run go test in verbose mode
ifeq (,$(CI))
GOTEST_OPTS=-race
else
GOTEST_OPTS=-race -v
endif

.PHONY: build
build: build-agent

.PHONY: build-agent
build-agent: otelcol-builder
	@mkdir -p _build/agent
	@$(eval AGENT_TMP := $(shell mktemp -d))
	@rm -rf ${AGENT_TMP}/agent.yaml
	@sed "s/version:.*/version: ${VERSION}/g" manifests/agent.yaml > ${AGENT_TMP}/agent.yaml
	$(OTELCOL_BUILDER) --otelcol-version ${OTELCOL_VERSION} --config ${AGENT_TMP}/agent.yaml

.PHONY: test
test: fmt
	go test ${GOTEST_OPTS} ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint: golangci-lint
	${GOLANGCI_LINT} run

.PHONY: golangci-lint
golangci-lint:
ifeq (, $(shell which golangci-lint))
	@{ \
	set -e ;\
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.38.0 ;\
	}
else
GOLANGCI_LINT=$(shell which golangci-lint)
endif

.PHONY: otelcol-builder
otelcol-builder:
	@scripts/install_otelcol_builder.sh -d $(OTELCOL_BUILDER_DIR) -v $(OTELCOL_BUILDER_VERSION)
