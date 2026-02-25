VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.Version=$(VERSION)
PREFIX   ?= /usr/local

.PHONY: build test cover lint mod-tidy snapshot release install install-service uninstall-service install-desktop clean help

## build: compile z13gui (CGO required for GTK4)
build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o z13gui .

## test: run unit tests (pure Go; no GTK4 headers required)
test:
	go test ./internal/theme/...

## cover: run tests with coverage report
cover:
	go test -coverprofile=coverage.out ./internal/theme/...
	go tool cover -func=coverage.out

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## mod-tidy: tidy go.mod
mod-tidy:
	go mod tidy

## snapshot: build a local snapshot release via goreleaser (no publish)
snapshot:
	goreleaser release --snapshot --clean

## release: publish a release via goreleaser (requires a clean git tag)
release:
	goreleaser release --clean

## install: install pre-built binary to PREFIX/bin (run make build first)
install:
	@test -f z13gui || { echo "error: z13gui binary not found. Run 'make build' first."; exit 1; }
	install -Dm755 z13gui $(DESTDIR)$(PREFIX)/bin/z13gui

## install-service: install and enable the z13gui systemd user service
install-service:
	install -Dm644 contrib/z13gui.service $(HOME)/.config/systemd/user/z13gui.service
	systemctl --user daemon-reload
	systemctl --user enable --now z13gui
	@echo "Service installed. Run 'systemctl --user status z13gui' to verify."

## uninstall-service: stop and remove the z13gui systemd user service
uninstall-service:
	-systemctl --user disable --now z13gui
	rm -f $(HOME)/.config/systemd/user/z13gui.service
	systemctl --user daemon-reload
	@echo "Service removed."

## install-desktop: install desktop entry for the current user
install-desktop:
	install -Dm644 contrib/z13gui.desktop $(HOME)/.local/share/applications/z13gui.desktop

## clean: remove all generated build and test artifacts
clean:
	rm -f z13gui
	rm -rf dist/
	find . -name '*.test' -delete
	find . -name 'coverage.out' -o -name 'coverage.*' -o -name '*.coverprofile' -o -name 'profile.cov' | xargs rm -f

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/^## /  /'
