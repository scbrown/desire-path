.PHONY: build test vet clean install

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
