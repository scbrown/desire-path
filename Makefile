.PHONY: build test vet clean install docs docs-serve

build:
	go build -o dp ./cmd/dp

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f dp

install:
	go install ./cmd/dp

docs:
	mdbook build docs/book

docs-serve:
	mdbook serve docs/book
