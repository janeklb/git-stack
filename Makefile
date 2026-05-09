GO ?= go
SHELL := /bin/bash
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/git-stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')
CI_TEST_GO_IMAGE ?= golang:1.22.12-bookworm
CI_TEST_GOCACHE ?= stack-ci-go-build-cache
CI_TEST_GOMODCACHE ?= stack-ci-go-mod-cache

.PHONY: test test-linux build install fmt clean

test:
	$(GO) test ./...

test-linux:
	docker run --rm -t \
		-v "$(CURDIR):/workspace" \
		-v "$(CI_TEST_GOCACHE):/root/.cache/go-build" \
		-v "$(CI_TEST_GOMODCACHE):/go/pkg/mod" \
		-w /workspace $(CI_TEST_GO_IMAGE) go test ./...

build: $(BINARY)

$(BINARY): go.mod $(GO_SRCS)
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) ./cmd/git-stack

install:
	@BIN_DIR="$$($(GO) env GOBIN)"; \
	if [ -z "$$BIN_DIR" ]; then BIN_DIR="$$($(GO) env GOPATH)/bin"; fi; \
	mkdir -p "$$BIN_DIR"; \
	$(GO) install ./cmd/git-stack; \
	printf "installed git-stack in %s\n" "$$BIN_DIR"; \
	printf "optional alias: alias stack=git-stack\n"

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf $(BIN_DIR)
