VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.Version=$(VERSION)
PREFIX   ?= /usr/local

.PHONY: build install install-service install-desktop lint clean snapshot release

## build: compile z13gui (CGO required for GTK4)
build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o z13gui .

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

## install-desktop: install desktop entry for the current user
install-desktop:
	install -Dm644 contrib/z13gui.desktop $(HOME)/.local/share/applications/z13gui.desktop

## snapshot: build release locally (no publish)
snapshot:
	goreleaser release --snapshot --clean

## release: build and publish release
release:
	goreleaser release --clean

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## clean: remove build artifact
clean:
	rm -f z13gui

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/^## /  /'
