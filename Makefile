GO ?= go
SHELL := /bin/bash
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/git-stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')
TEST_LINUX_GO_IMAGE ?= golang:1.22.12-bookworm
TEST_LINUX_GOCACHE ?= stack-test-linux-go-build-cache
TEST_LINUX_GOMODCACHE ?= stack-test-linux-go-mod-cache

.PHONY: test test-linux build install fmt clean

test:
	$(GO) test ./...

test-linux:
	docker run --rm -t \
		-v "$(CURDIR):/workspace" \
		-v "$(TEST_LINUX_GOCACHE):/root/.cache/go-build" \
		-v "$(TEST_LINUX_GOMODCACHE):/go/pkg/mod" \
		-w /workspace $(TEST_LINUX_GO_IMAGE) go test ./...

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
