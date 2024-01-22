$(GOBIN)/golangci-lint:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2

$(GOBIN)/gofumpt:
	$(GO) install mvdan.cc/gofumpt@latest

$(GOBIN)/goversioninfo:
	$(GO) install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0

 $(GOBIN)/mockery:
	$(GO) install github.com/vektra/mockery/v2@v2.14.0
