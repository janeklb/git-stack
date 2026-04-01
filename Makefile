GO ?= go
SHELL := /bin/bash
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')

.PHONY: test test-timings build install fmt clean

test:
	$(GO) test ./...

test-timings:
	@set -o pipefail; \
	$(GO) test -json ./... | jq -r -s 'map(select((.Action=="pass" or .Action=="fail") and (.Test != null))) | sort_by(.Elapsed // 0) | reverse[] | [.Action, (.Elapsed // 0), (.Package // ""), .Test] | @tsv'

build: $(BINARY)

$(BINARY): go.mod $(GO_SRCS)
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) ./cmd/stack

install:
	@BIN_DIR="$$($(GO) env GOBIN)"; \
	if [ -z "$$BIN_DIR" ]; then BIN_DIR="$$($(GO) env GOPATH)/bin"; fi; \
	mkdir -p "$$BIN_DIR"; \
	$(GO) install ./cmd/stack; \
	ln -sf "$$BIN_DIR/stack" "$$BIN_DIR/git-stack"; \
	printf "installed stack and git-stack in %s\n" "$$BIN_DIR"

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf $(BIN_DIR)
