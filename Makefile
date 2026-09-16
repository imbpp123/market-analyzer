GOLANGCI_LINT_VERSION := v2.13.2
TOOLS_DIR := $(CURDIR)/.bin
GOLANGCI_LINT_DIR := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT := $(GOLANGCI_LINT_DIR)/golangci-lint
PROTOC_GEN_GO_VERSION := v1.36.10
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1
PROTO_INCLUDE ?= $(dir $(shell command -v protoc))../include

.PHONY: fmt generate-tools generate build vet test race lint check

fmt:
	gofmt -w internal

generate-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

generate:
	mkdir -p api/go/marketanalyzer/v1
	PATH="$$(go env GOPATH)/bin:$$PATH" protoc -I api/proto/marketanalyzer/v1 -I "$(PROTO_INCLUDE)" \
		--go_out=api/go/marketanalyzer/v1 --go_opt=paths=source_relative \
		--go-grpc_out=api/go/marketanalyzer/v1 --go-grpc_opt=paths=source_relative \
		api/proto/marketanalyzer/v1/market_analyzer.proto

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

$(GOLANGCI_LINT):
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOLANGCI_LINT_DIR) $(GOLANGCI_LINT_VERSION)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

check: build vet test race lint
	test -z "$$(gofmt -l internal)"
	git diff --check
