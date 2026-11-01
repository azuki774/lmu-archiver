GO ?= go

.PHONY: build run mock test test-e2e

build:
	$(GO) build -trimpath -o bin/ ./cmd/lmu-archiver ./cmd/mock-apis

run:
	$(GO) run ./cmd/lmu-archiver $(ARGS)

mock:
	$(GO) run ./cmd/mock-apis $(ARGS)

test:
	$(GO) test ./...

test-e2e:
	$(GO) test -tags=e2e -count=1 -timeout=2m -v ./e2e
