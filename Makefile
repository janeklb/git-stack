GO ?= go

.PHONY: test build build-stack build-git-stack fmt

test:
	$(GO) test ./...

build: build-stack build-git-stack

build-stack:
	$(GO) build -o stack ./cmd/stack

build-git-stack: build-stack
	ln -sf stack git-stack

fmt:
	gofmt -w ./cmd ./internal
