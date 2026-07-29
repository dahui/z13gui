VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.Version=$(VERSION)
PREFIX   ?= /usr/local

HIDBLOCKER_DIR := internal/gui/gamepad/hidblocker

# PURE_PKGS is every internal package that compiles without CGO and GTK4 headers,
# derived rather than hand-listed so a newly added package is tested automatically.
# `go test ./...` cannot be used because internal/gui needs both; `go list` only
# reads the source, so it works without them. Anything under internal/gui is
# excluded by construction — that is the boundary: widgets there, decisions here.
PURE_PKGS := $(shell go list ./internal/... 2>/dev/null | grep -v '/internal/gui')

.PHONY: build test race cover lint fmt-check mod-tidy vmlinux generate snapshot release install setcap install-service uninstall-service install-desktop clean help

## build: compile z13gui (CGO required for GTK4)
build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o z13gui .

## test: run unit tests (pure Go; no GTK4 headers required)
test:
	go test $(PURE_PKGS)

## race: run unit tests under the race detector
race:
	go test -race $(PURE_PKGS)

## cover: run tests with coverage report
cover:
	go test -coverprofile=coverage.out $(PURE_PKGS)
	go tool cover -func=coverage.out

## lint: check formatting, then run golangci-lint
lint: fmt-check
	golangci-lint run ./...

## fmt-check: fail if any file needs gofmt
#
# gofmt is not among golangci-lint's enabled linters, so `make lint` alone cannot
# see formatting — which made a gofmt slip discoverable only after a push, from
# CI. The CI job calls this target rather than carrying its own copy of the
# command, so local and CI agree by construction instead of by remembering to
# update both. The generated bpf2go bindings are excluded: they are committed as
# the tool emits them.
fmt-check:
	@unformatted="$$(gofmt -l . | grep -v '^$(HIDBLOCKER_DIR)/blocker_' || true)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; \
		echo "$$unformatted"; \
		echo "Run: gofmt -w <file>"; \
		exit 1; \
	fi

## mod-tidy: tidy go.mod
mod-tidy:
	go mod tidy

## vmlinux: generate vmlinux.h from kernel BTF (requires bpftool)
vmlinux: $(HIDBLOCKER_DIR)/vmlinux.h

$(HIDBLOCKER_DIR)/vmlinux.h:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@

## generate: compile BPF program and generate Go bindings (requires clang)
generate: vmlinux
	cd $(HIDBLOCKER_DIR) && go generate

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

## setcap: grant BPF capabilities to installed binary (enables hidraw blocker)
setcap:
	sudo setcap cap_bpf,cap_perfmon+ep $(DESTDIR)$(PREFIX)/bin/z13gui

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
