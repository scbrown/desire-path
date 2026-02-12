.PHONY: build test integration-test vet clean install docs docs-serve

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null)
LDFLAGS  = -X github.com/scbrown/desire-path/internal/cli.Version=$(VERSION) \
           -X github.com/scbrown/desire-path/internal/cli.Commit=$(COMMIT)

build:
	go build -ldflags "$(LDFLAGS)" -o dp ./cmd/dp

test:
	go test ./...

integration-test:
	go test -tags integration ./internal/integration/

vet:
	go vet ./...

clean:
	rm -f dp

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/dp

docs:
	mdbook build docs/book

docs-serve:
	mdbook serve docs/book
