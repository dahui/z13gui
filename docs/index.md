# z13gui

GTK4 overlay drawer for **z13ctl** on Wayland — graphical controls for the
2025 ASUS ROG Flow Z13 on Linux.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/dahui/z13gui/blob/main/LICENSE)

---

## What z13gui does

- **Profile switching** — quiet, balanced, and performance TDP profiles
- **Battery charge limit** — set the charge cap (40–100%) from the drawer
- **RGB lighting** — mode, color, speed, and brightness for the keyboard
  backlight and edge lightbar
- **System toggles** — panel overdrive and boot sound on/off
- **Theme picker** — 15 built-in themes with full custom theme support
- **Gamepad navigation** — full D-pad + button control for Steam Gaming Mode

All hardware communication goes through the z13ctl daemon. z13gui never
touches HID devices or sysfs directly.

---

## Display backends

Two backends are supported, selected automatically based on the session
environment:

- **Layer-shell** (KDE Plasma, Hyprland, Sway) — Wayland layer-shell overlay
  with margin-based slide animation and focus-loss dismiss
- **Gamescope** (Steam Gaming Mode) — X11 overlay via the `STEAM_OVERLAY` atom
  with opacity-based visibility and a click-to-dismiss backdrop

---

## Requirements

- Wayland compositor with layer-shell support, or gamescope (Steam Gaming Mode)
- GTK 4 and gtk4-layer-shell libraries (see [Installation](installation.md#runtime-dependencies) for distro package names)
- [z13ctl](https://github.com/dahui/z13ctl) daemon running

---

## Next steps

- [**Installation**](installation.md) — download the binary or build from source
- [**Quick Start**](getting-started.md) — open the drawer and explore the controls
- [**Configuration**](configuration.md) — config file, environment variables
- [**Theming**](theming.md) — built-in themes and custom color definitions
