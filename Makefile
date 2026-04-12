GO ?= go
SHELL := /bin/bash
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')
CI_TEST_IMAGE ?= stack-ci-test
CI_TEST_GOCACHE ?= stack-ci-go-build-cache
CI_TEST_GOMODCACHE ?= stack-ci-go-mod-cache
RAMDISK_SECTORS ?= 2097152

.PHONY: test test-timings test-command-timings test-linux test-linux-timings test-linux-tmpfs test-ramdisk build-ci-test-image build install fmt clean

test:
	$(GO) test ./...

test-timings:
	@set -o pipefail; \
	$(GO) test -json ./... | jq -r -s 'map(select((.Action=="pass" or .Action=="fail") and (.Test != null))) | sort_by(.Elapsed // 0) | reverse[] | [.Action, (.Elapsed // 0), (.Package // ""), .Test] | @tsv'

test-command-timings:
	@summary_file="$$(mktemp)"; \
	STACK_TEST_COMMAND_TIMING=1 STACK_TEST_COMMAND_TIMING_SUMMARY="$$summary_file" $(GO) test -count=1 ./internal/app; \
	status=$$?; \
	cat "$$summary_file"; \
	rm -f "$$summary_file"; \
	exit $$status

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

test-linux-tmpfs: build-ci-test-image
	docker run --rm -t \
		-v "$(CURDIR):/workspace" \
		-v "$(CI_TEST_GOCACHE):/root/.cache/go-build" \
		-v "$(CI_TEST_GOMODCACHE):/go/pkg/mod" \
		--tmpfs /tmp/stack-tests:exec,mode=1777 \
		-e TMPDIR=/tmp/stack-tests \
		-w /workspace $(CI_TEST_IMAGE) make test

test-ramdisk:
	@if [ "$$(uname -s)" != "Darwin" ]; then \
		printf 'test-ramdisk is only supported on macOS\n' >&2; \
		exit 1; \
	fi
	@set -euo pipefail; \
	device=""; \
	volume_name="stack-test-ramdisk-$$$$"; \
	volume_dir="/Volumes/$$volume_name"; \
	cleanup() { \
		status=$$?; \
		if [ -n "$$device" ]; then \
			hdiutil detach "$$device" >/dev/null 2>&1 || true; \
		fi; \
		exit $$status; \
	}; \
	trap cleanup EXIT INT TERM; \
	device="$$(hdiutil attach -nomount ram://$(RAMDISK_SECTORS) | awk 'NR==1 {print $$1}')"; \
	diskutil erasevolume HFS+ "$$volume_name" "$$device" >/dev/null; \
	tmp_dir="$$volume_dir/tmp"; \
	mkdir -p "$$tmp_dir"; \
	printf 'using RAM disk at %s\n' "$$tmp_dir"; \
	TMPDIR="$$tmp_dir" $(MAKE) test

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
