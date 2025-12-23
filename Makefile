.PHONY: build test clean install lint fmt

BINARY=docmap

build:
	go build -o $(BINARY) .

test:
	go test -v ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) coverage.out coverage.html

install: build
	mv $(BINARY) /usr/local/bin/

lint:
	go vet ./...
	staticcheck ./...

fmt:
	gofmt -w .

run: build
	./$(BINARY) .

# Quick dev cycle
dev: fmt lint test build
