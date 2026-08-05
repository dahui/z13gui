# z13gui

GTK4 overlay drawer for **z13ctl** on Wayland — graphical controls for the
2025 ASUS ROG Flow Z13 on Linux.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/dahui/z13gui/blob/main/LICENSE)

---

## What z13gui does

- **Profile switching** — quiet, balanced, performance, and custom TDP profiles
- **Custom TDP control** — configurable power limits (PL1/PL2/PL3) in the custom profile view, with basic and advanced modes
- **Fan curve editor** — per-profile fan response curve editing (custom profile, advanced mode)
- **Undervolt** — CPU Curve Optimizer offset (requires `ryzen_smu` kernel module; iGPU CO is not supported on Strix Halo)
- **APU telemetry** — live temperature and fan RPM readouts in the custom profile view
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

Three backends are supported, selected automatically based on the session
environment:

- **Layer-shell** (KDE Plasma, Hyprland, Sway) — Wayland layer-shell overlay
  with margin-based slide animation and focus-loss dismiss
- **Overlay** (GNOME, and any compositor without layer-shell) — a fullscreen,
  transparent window with the drawer against the right edge. Everything outside
  the drawer is click-through, so the rest of the desktop stays usable
- **Gamescope** (Steam Gaming Mode) — X11 overlay via the `STEAM_OVERLAY` atom
  with opacity-based visibility and a click-to-dismiss backdrop

!!! note "Why GNOME needs a different backend"

    The layer-shell protocol (`zwlr_layer_shell_v1`) is a wlroots extension, not
    part of `wayland-protocols`. KWin, Hyprland and Sway implement it; GNOME's
    Mutter never has, taking the position that panels belong in Shell extensions.
    Installing `gtk4-layer-shell` does not change this — that library is the
    *client* side, and the protocol has to come from the compositor. z13gui
    detects this at startup and uses the overlay backend instead.

---

## Requirements

- A Wayland compositor (layer-shell is used when available; GNOME and others get
  the overlay backend), or gamescope (Steam Gaming Mode)
- GTK 4 and gtk4-layer-shell libraries (see [Installation](installation.md#runtime-dependencies) for distro package names)
- [z13ctl](https://github.com/dahui/z13ctl) daemon running

---

## Next steps

- [**Installation**](installation.md) — download the binary or build from source
- [**Quick Start**](getting-started.md) — open the drawer and explore the controls
- [**Configuration**](configuration.md) — config file, environment variables
- [**Theming**](theming.md) — built-in themes and custom color definitions
