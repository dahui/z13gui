# z13gui

GTK4 overlay drawer for [z13ctl](https://github.com/dahui/z13ctl) on Wayland.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Table of Contents

- [Background](#background)
- [Requirements](#requirements)
- [Installation](#installation)
- [Running as a Service](#running-as-a-service)
- [Usage](#usage)
- [Themes](#themes)
- [Command-line Flags](#command-line-flags)
- [Configuration](#configuration)
- [Contributing](#contributing)

## Background

`z13ctl` provides full RGB lighting, performance profile, and battery charge
limit control for the 2025 ASUS ROG Flow Z13 on Linux. It works entirely from
the command line and through a background daemon.

`z13gui` is the graphical companion. It renders a slide-out drawer on the right
edge of the screen, triggered by the Armoury Crate button (KEY_PROG3). The
daemon broadcasts a `gui-toggle` event over its subscribe socket, and z13gui
listens for it. All hardware communication goes through z13ctl -- the GUI never
touches HID devices or sysfs directly.

## Requirements

- Wayland compositor (tested on KDE Plasma and Hyprland)
- `gtk4-layer-shell` library (pkg-config: `gtk4-layer-shell-0`)
- `z13ctl` daemon running (see [z13ctl installation](https://github.com/dahui/z13ctl#installation))

## Installation

**Pre-built binaries** are available on the
[Releases](../../releases) page.

```sh
tar xzf z13gui_*_linux_amd64.tar.gz
sudo install -Dm755 z13gui /usr/local/bin/z13gui
```

Make sure `z13ctl` is installed and its daemon is running before launching
z13gui. See the [z13ctl README](https://github.com/dahui/z13ctl#installation)
for setup instructions.

**From source:**

Requires Go 1.22+, CGO enabled, and the `gtk4-layer-shell` C library.

```sh
git clone https://github.com/dahui/z13gui
cd z13gui
make build
sudo install -Dm755 z13gui /usr/local/bin/z13gui
```

## Running as a Service

z13gui is designed to run in the background, waiting for the Armoury Crate
button press. A systemd user service is the recommended way to start it
automatically on login.

**From source:**

```sh
make install
make install-service
systemctl --user enable --now z13gui
```

**From binary release:**

```sh
sudo install -Dm755 z13gui /usr/local/bin/z13gui
install -Dm644 dist/z13gui.service ~/.config/systemd/user/z13gui.service
systemctl --user daemon-reload
systemctl --user enable --now z13gui
```

To install the desktop entry (optional):

```sh
# From source:
make install-desktop

# From binary release:
install -Dm644 dist/z13gui.desktop ~/.local/share/applications/z13gui.desktop
```

Check service status:

```sh
systemctl --user status z13gui
journalctl --user -u z13gui -f
```

## Usage

Press the Armoury Crate button on your Z13 to open the drawer. Press it again
(or click outside the drawer) to close it.

The drawer provides controls for:

- **Profile** -- switch between `quiet`, `balanced`, and `performance` TDP
  profiles
- **Battery Limit** -- set the battery charge limit (40-100%)
- **Keyboard / Lightbar** -- tab between the two lighting zones
- **Mode** -- choose a lighting effect: static, breathe, cycle, rainbow,
  strobe, or off
- **Color** -- pick from 8 preset colors or open a custom color chooser
- **Speed** -- animation speed for modes that support it (slow, normal, fast)
- **Brightness** -- lighting brightness (0-3)

Changes are sent to the z13ctl daemon immediately and persist across reboots.

## Themes

z13gui ships with 15 built-in themes (8 dark, 7 light). A theme picker button
at the bottom of the drawer lets you switch between them. Catppuccin themes
also support 14 accent color variants.

You can create your own theme by writing a simple TOML file with 7 color
values. See [THEMING.md](THEMING.md) for the full theming guide.

## Command-line Flags

| Flag | Description |
|------|-------------|
| `--verbose`, `-v` | Enable debug logging (includes GTK messages) |
| `--version` | Print version and exit |
| `--print-theme` | Print the default theme.toml to stdout |
| `--list-themes` | List all built-in themes and exit |

`--print-theme` is useful for bootstrapping a custom theme:

```sh
z13gui --print-theme > ~/.config/z13gui/theme.toml
```

## Configuration

z13gui stores its configuration in `~/.config/z13gui/config.toml`. This file
is updated automatically when you use the theme picker.

```toml
theme = "catppuccin-mocha"
accent = "sapphire"
```

| Key | Description |
|-----|-------------|
| `theme` | Built-in theme ID (see `--list-themes`) |
| `accent` | Accent color variant for themes that support it |

If no config file exists, z13gui defaults to the `rog-dark` theme.

## Contributing

Contributions are welcome. Please open an issue before starting work on a
significant change so the approach can be discussed first.

**Setup:**

```sh
git clone https://github.com/dahui/z13gui
cd z13gui
```

During development, if you need to work against a local copy of the z13ctl API
module, create a `go.work` file (it is gitignored):

```sh
go work init . ../z13rgb/api
```

**Before submitting a pull request:**

```sh
make build
make lint
go test ./internal/theme/ -v
```

Pull requests should pass both `make lint` and all tests without errors.
