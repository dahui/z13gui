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
- [Environment Variables](#environment-variables)
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

Two display backends are supported:

- **Layer-shell** (KDE Plasma, Hyprland, Sway) -- Wayland layer-shell overlay
  with margin-based slide animation
- **Gamescope** (Steam Gaming Mode) -- X11 overlay via `STEAM_OVERLAY` atom
  with opacity-based visibility

The backend is selected automatically based on the session environment.

## Requirements

- Wayland compositor with layer-shell support (tested on KDE Plasma), or
  gamescope (Steam Gaming Mode)
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

Requires Go 1.23+, CGO enabled, and GTK4 development libraries.

Build dependencies (Debian/Ubuntu):

```sh
sudo apt-get install -y libgtk-4-dev libgtk4-layer-shell-dev
```

Build dependencies (Arch Linux):

```sh
sudo pacman -S gtk4 gtk4-layer-shell
```

Then build and install:

```sh
git clone https://github.com/dahui/z13gui
cd z13gui
make build
sudo make install
```

## Running as a Service

z13gui is designed to run in the background, waiting for the Armoury Crate
button press. A systemd user service is the recommended way to start it
automatically on login.

**From source:**

```sh
make build
sudo make install
make install-service
```

**From binary release:**

```sh
sudo install -Dm755 z13gui /usr/local/bin/z13gui
install -Dm644 contrib/z13gui.service ~/.config/systemd/user/z13gui.service
systemctl --user daemon-reload
systemctl --user enable --now z13gui
```

To install the desktop entry (optional):

```sh
# From source:
make install-desktop

# From binary release:
install -Dm644 contrib/z13gui.desktop ~/.local/share/applications/z13gui.desktop
```

Check service status:

```sh
systemctl --user status z13gui
journalctl --user -u z13gui -f
```

## Usage

Press the Armoury Crate button on your Z13 to open the drawer. Press it again
(or click outside the drawer, or press Escape) to close it.

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
- **Panel Overdrive** -- toggle panel overdrive for faster pixel response
- **Boot Sound** -- enable or disable the startup sound

Changes are sent to the z13ctl daemon immediately and persist across reboots.

### Gamepad Controls

z13gui supports gamepad/controller navigation for use in Steam Gaming Mode:

- **D-pad** -- navigate between controls
- **A (Cross)** -- activate buttons/switches or enter edit mode for sliders
- **B (Circle)** -- cancel edit, go back, or close the drawer
- **Shoulder buttons** -- jump between sections

Gamepad focus is automatically hidden when the mouse moves. Set
`Z13GUI_NO_GAMEPAD=1` to disable gamepad input entirely.

### Gamescope (Steam Gaming Mode)

In Steam Gaming Mode, z13gui runs as a gamescope X11 overlay. Popups and
dropdowns are replaced with full-view alternatives (theme picker view, HSL
color picker view) because gamescope does not composite separate popup windows.

The UI is automatically scaled based on the output resolution. Use the
`Z13GUI_SCALE` environment variable to override the auto-detected scale factor.

## Themes

z13gui ships with 15 built-in themes (8 dark, 7 light). A theme picker button
at the bottom of the drawer lets you switch between them. Catppuccin themes
also support 14 accent color variants.

You can create your own theme by writing a simple TOML file with 7 color
values. See [THEMING.md](THEMING.md) for the full theming guide.

## Command-line Flags

| Flag | Description |
|------|-------------|
| `--debug`, `-d` | Enable debug logging (includes GTK messages) |
| `--version` | Print version and exit |
| `--print-theme` | Print the default theme.toml to stdout |
| `--list-themes` | List all built-in themes and exit |

`--print-theme` is useful for bootstrapping a custom theme:

```sh
z13gui --print-theme > ~/.config/z13gui/theme.toml
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `Z13GUI_SCALE` | Override CSS scale factor in gamescope mode (default: auto-detected from output resolution) |
| `Z13GUI_NO_GAMEPAD` | Set to `1` to disable gamepad input |

z13gui also sets `GTK_A11Y=none` internally to disable the GTK4 accessibility
bridge. This prevents D-Bus timeouts when running under systemd where the
AT-SPI bus may be unavailable.

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
go work init . ../z13ctl/api
```

**Before submitting a pull request:**

```sh
make build
make lint
go test ./internal/theme/ -v
```

Pull requests should pass both `make lint` and all tests without errors.
