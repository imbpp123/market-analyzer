GOLANGCI_LINT_VERSION := v2.13.2
TOOLS_DIR := $(CURDIR)/.bin
GOLANGCI_LINT_DIR := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT := $(GOLANGCI_LINT_DIR)/golangci-lint
PROTOC_GEN_GO_VERSION := v1.36.10
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1
PROTO_TOOLS_DIR := $(TOOLS_DIR)/protobuf-$(PROTOC_GEN_GO_VERSION)-$(PROTOC_GEN_GO_GRPC_VERSION)
PROTOC_GEN_GO := $(PROTO_TOOLS_DIR)/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(PROTO_TOOLS_DIR)/protoc-gen-go-grpc
PROTO_INCLUDE ?= $(dir $(shell command -v protoc))../include

.PHONY: fmt generate-tools generate generate-check build vet test race benchmark validate-live load-live lint check

VALIDATION_CONFIG ?= docs/validation/config.local.json
VALIDATION_REPORT ?= validation-report.json
LOAD_REPORT ?= load-report.json
LOAD_METRICS_URL ?= http://127.0.0.1:8081/metrics
LOAD_CONCURRENCY ?= 8
LOAD_REQUESTS ?= 200

fmt:
	gofmt -w cmd internal

generate-tools: $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)

$(PROTOC_GEN_GO):
	mkdir -p $(PROTO_TOOLS_DIR)
	GOBIN=$(PROTO_TOOLS_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)

$(PROTOC_GEN_GO_GRPC):
	mkdir -p $(PROTO_TOOLS_DIR)
	GOBIN=$(PROTO_TOOLS_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

generate: generate-tools
	mkdir -p api/go/marketanalyzer/v1
	PATH="$(PROTO_TOOLS_DIR):$$PATH" protoc -I api/proto/marketanalyzer/v1 -I "$(PROTO_INCLUDE)" \
		--go_out=api/go/marketanalyzer/v1 --go_opt=paths=source_relative \
		--go-grpc_out=api/go/marketanalyzer/v1 --go-grpc_opt=paths=source_relative \
		api/proto/marketanalyzer/v1/market_analyzer.proto

generate-check: generate-tools
	set -e; tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT; \
	PATH="$(PROTO_TOOLS_DIR):$$PATH" protoc -I api/proto/marketanalyzer/v1 -I "$(PROTO_INCLUDE)" \
		--go_out="$$tmpdir" --go_opt=paths=source_relative \
		--go-grpc_out="$$tmpdir" --go-grpc_opt=paths=source_relative \
		api/proto/marketanalyzer/v1/market_analyzer.proto; \
	diff -u api/go/marketanalyzer/v1/market_analyzer.pb.go "$$tmpdir/market_analyzer.pb.go"; \
	diff -u api/go/marketanalyzer/v1/market_analyzer_grpc.pb.go "$$tmpdir/market_analyzer_grpc.pb.go"

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

benchmark:
	go test ./internal/domain ./internal/transport/grpc -run '^$$' \
		-bench 'Benchmark(Calculations|FullResponseConstruction)$$' -benchmem -count=5

validate-live:
	go run ./cmd/market-analyzer-validate -config "$(VALIDATION_CONFIG)" -output "$(VALIDATION_REPORT)"

load-live:
	go run ./cmd/market-analyzer-load -config "$(VALIDATION_CONFIG)" -output "$(LOAD_REPORT)" \
		-metrics-url "$(LOAD_METRICS_URL)" -concurrency "$(LOAD_CONCURRENCY)" -requests "$(LOAD_REQUESTS)"

$(GOLANGCI_LINT):
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOLANGCI_LINT_DIR) $(GOLANGCI_LINT_VERSION)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

check: build vet test race lint
	test -z "$$(gofmt -l cmd internal)"
	git diff --check
