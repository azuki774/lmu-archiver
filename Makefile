GO ?= go

.PHONY: build run test

build:
	$(GO) build -trimpath -o bin/ ./cmd/lmu-archiver

run:
	$(GO) run ./cmd/lmu-archiver $(ARGS)

test:
	$(GO) test ./...
