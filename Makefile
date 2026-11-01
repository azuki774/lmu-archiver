GO ?= go

.PHONY: build run mock test

build:
	$(GO) build -trimpath -o bin/ ./cmd/lmu-archiver ./cmd/mock-apis

run:
	$(GO) run ./cmd/lmu-archiver $(ARGS)

mock:
	$(GO) run ./cmd/mock-apis $(ARGS)

test:
	$(GO) test ./...
