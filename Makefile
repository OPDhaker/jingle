BIN     := bin/jingle
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/OPDhaker/jingle/internal/cli.version=$(VERSION)

.PHONY: build test lint fmt clean gui-dev gui-build gui-test gui-icons

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

# Renders the app icons from gui/assets/icon.svg (bundle) and tray.svg (menu bar).
# Needs ImageMagick. Keeps only the icon files tauri.conf.json lists, plus tray.png.
gui-icons:
	cd gui && pnpm install && rm -rf .icons && pnpm tauri icon assets/icon.svg -o .icons
	cd gui && cp .icons/32x32.png .icons/64x64.png .icons/128x128.png .icons/128x128@2x.png .icons/icon.icns .icons/icon.png src-tauri/icons/ && rm -rf .icons
	magick -background none -density 1536 gui/assets/tray.svg -resize 64x64 PNG32:gui/src-tauri/icons/tray.png

clean:
	rm -rf bin dist gui/dist
