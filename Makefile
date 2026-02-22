VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.Version=$(VERSION)

.PHONY: build install lint clean

## build: compile z13gui (CGO required for GTK4)
build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o z13gui .

## install: build and install to /usr/local/bin
install: build
	install -Dm755 z13gui /usr/local/bin/z13gui

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## clean: remove build artifact
clean:
	rm -f z13gui

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/^## /  /'
