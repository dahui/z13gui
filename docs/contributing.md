# Contributing

Contributions are welcome. Please open an issue before starting work on a
significant change so the approach can be discussed first.

---

## Repository structure

Single Go module (`github.com/dahui/z13gui`).

| Package | Purpose |
|---------|---------|
| `internal/gui` | Main Window type, daemon subscription, gamepad navigation |
| `internal/gui/layershell` | Wayland layer-shell display backend |
| `internal/gui/gamescope` | X11 overlay backend for Steam Gaming Mode |
| `internal/gui/gamepad` | Linux evdev gamepad reader |
| `internal/gui/fonts` | Embedded Inter font registration |
| `internal/theme` | Color definitions, TOML parsing, CSS generation — pure Go |

---

## Development setup

```sh
git clone https://github.com/dahui/z13gui
cd z13gui
```

**Build dependencies (Arch Linux):**

```sh
sudo pacman -S gtk4 gtk4-layer-shell
```

**Build dependencies (Debian/Ubuntu):**

```sh
sudo apt-get install -y libgtk-4-dev libgtk4-layer-shell-dev
```

To work against a local copy of the z13ctl API module, create a `go.work` file
(it is gitignored):

```sh
go work init . ../z13ctl/api
```

---

## Before submitting a pull request

```sh
make build     # compile (requires GTK4 headers)
make lint      # run golangci-lint
make test      # run unit tests (pure Go, no GTK4 required)
```

Tests are in `internal/theme` (pure Go, no hardware or GTK4 dependency). GUI
packages are integration-tested manually against hardware.

Pull requests must pass both `make build` and `make lint` without errors, and
should include tests for any changes to `internal/theme`.

---

## Testing notes

- `internal/theme` — fully unit-testable; covers color parsing, CSS generation,
  config persistence, and all 78 built-in theme/accent combinations
- `internal/gui` — requires GTK4; integration-tested manually against hardware
- Display backends (layershell, gamescope) — require a compositor or gamescope;
  no automated tests

---

## Release workflow (maintainers only)

```sh
git tag vX.Y.Z && git push origin vX.Y.Z
```

GoReleaser handles binary builds, the Arch `.pkg.tar.zst` package, and GitHub
Release creation automatically when the tag is pushed.
