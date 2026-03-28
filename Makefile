GO ?= go
BIN_DIR ?= bin

.PHONY: test build build-stack build-git-stack fmt

test:
	$(GO) test ./...

build: build-stack build-git-stack

build-stack:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/stack ./cmd/stack

build-git-stack: build-stack
	ln -sf stack $(BIN_DIR)/git-stack

fmt:
	gofmt -w ./cmd ./internal
