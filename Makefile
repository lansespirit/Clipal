.PHONY: build test test-unit test-smoke lint vuln fmt ci

GO ?= go
GOBIN := $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo "$(GOBIN)/golangci-lint")
GOVULNCHECK ?= $(shell command -v govulncheck 2>/dev/null || echo "$(GOBIN)/govulncheck")

build:
	./scripts/build.sh

test: test-unit

test-unit:
	$(GO) test ./...

test-smoke:
	./scripts/smoke_test.sh

lint:
	$(GOLANGCI_LINT) run ./...

vuln:
	$(GOVULNCHECK) ./...

fmt:
	gofmt -w $$(find cmd internal -type f -name '*.go' | sort)

ci: test-unit lint vuln test-smoke
