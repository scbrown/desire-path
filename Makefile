.PHONY: build test vet clean

build:
	go build -o dp ./cmd/dp

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f dp
