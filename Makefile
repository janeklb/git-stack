GO ?= go
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/stack
GO_SRCS := $(shell find cmd internal -name '*.go' -type f ! -name '*_test.go')

.PHONY: test test-timings build install fmt clean

test:
	$(GO) test ./...

test-timings:
	@tmp=$$(mktemp); \
	$(GO) test -json ./... > "$$tmp"; \
	status=$$?; \
	jq -r -s 'def fmt2: ((. // 0) * 100 | round / 100) as $$n | ($$n | tostring) as $$s | if ($$s | contains(".")) then ($$s | split(".") | .[0]) + "." + (((( $$s | split(".") | .[1]) + "00")[0:2])) else $$s + ".00" end; map(select((.Action=="pass" or .Action=="fail") and (.Test != null))) | sort_by(.Elapsed // 0) | reverse[] | [.Action, (.Elapsed | fmt2), (.Package // ""), .Test] | @tsv' "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status

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
