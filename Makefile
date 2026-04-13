GO ?= go
SHELL := /bin/bash
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/git-stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')
CI_TEST_IMAGE ?= stack-ci-test
CI_TEST_GOCACHE ?= stack-ci-go-build-cache
CI_TEST_GOMODCACHE ?= stack-ci-go-mod-cache

.PHONY: test test-timings test-linux test-linux-timings build-ci-test-image build install fmt clean

test:
	$(GO) test ./...

test-timings:
	@set -o pipefail; \
	$(GO) test -json ./... | jq -r -s 'map(select((.Action=="pass" or .Action=="fail") and (.Test != null))) | sort_by(.Elapsed // 0) | reverse[] | [.Action, (.Elapsed // 0), (.Package // ""), .Test] | @tsv'

build-ci-test-image:
	docker build -f build/ci-test.Dockerfile -t $(CI_TEST_IMAGE) .

test-linux: build-ci-test-image
	docker run --rm -t \
		-v "$(CURDIR):/workspace" \
		-v "$(CI_TEST_GOCACHE):/root/.cache/go-build" \
		-v "$(CI_TEST_GOMODCACHE):/go/pkg/mod" \
		-w /workspace $(CI_TEST_IMAGE) make test

test-linux-timings: build-ci-test-image
	docker run --rm -t \
		-v "$(CURDIR):/workspace" \
		-v "$(CI_TEST_GOCACHE):/root/.cache/go-build" \
		-v "$(CI_TEST_GOMODCACHE):/go/pkg/mod" \
		-w /workspace $(CI_TEST_IMAGE) make test-timings

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
