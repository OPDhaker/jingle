BIN     := bin/jingle
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/OPDhaker/jingle/internal/cli.version=$(VERSION)

.PHONY: build test lint fmt clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/jingle

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

clean:
	rm -rf bin dist
