BIN     := bin/jingle
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/OPDhaker/jingle/internal/cli.version=$(VERSION)

.PHONY: build test lint fmt clean gui-dev gui-build gui-test

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/jingle

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

# The menu-bar app (gui/). It runs the jingle on PATH, or $JINGLE_BIN.
gui-dev:
	cd gui && pnpm install && pnpm tauri dev

gui-build:
	cd gui && pnpm install && pnpm tauri build

gui-test:
	cd gui && pnpm install && pnpm build
	cd gui/src-tauri && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test

clean:
	rm -rf bin dist gui/dist
