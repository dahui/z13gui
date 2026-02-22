# Theming

z13gui supports custom color themes through a simple TOML configuration file.
You can also select from 15 built-in themes using the in-app theme picker.

## Table of Contents

- [Overview](#overview)
- [Built-in Themes](#built-in-themes)
- [Choosing a Built-in Theme](#choosing-a-built-in-theme)
- [Creating a Custom Theme](#creating-a-custom-theme)
  - [Accent Variants](#accent-variants)
- [Color Reference](#color-reference)
- [Accent Colors](#accent-colors)
- [Full CSS Override](#full-css-override)
- [Example](#example)

## Overview

z13gui resolves its theme using the following priority chain. The first match
wins:

1. `~/.config/z13gui/theme.toml` -- custom color definitions (7 hex values)
2. `~/.config/z13gui/theme.css` -- full CSS override (power users)
3. `~/.config/z13gui/config.toml` `theme = "id"` -- built-in theme selection
4. Compiled-in default (ROG Dark)

If you create a `theme.toml` file, it always takes priority over the built-in
theme picker selection. Delete or rename it to go back to built-in themes.

## Built-in Themes

| ID | Name | Type | Accents |
|----|------|------|---------|
| `catppuccin-frappe` | Catppuccin Frappe | dark | 14 variants |
| `catppuccin-latte` | Catppuccin Latte | light | 14 variants |
| `catppuccin-macchiato` | Catppuccin Macchiato | dark | 14 variants |
| `catppuccin-mocha` | Catppuccin Mocha | dark | 14 variants |
| `everforest-light` | Everforest Light | light | no |
| `github-light` | GitHub Light | light | no |
| `gruvbox-dark` | Gruvbox Dark | dark | no |
| `gruvbox-light` | Gruvbox Light | light | no |
| `nord` | Nord | dark | no |
| `one-light` | One Light | light | no |
| `rog-dark` | ROG Dark | dark | no |
| `rog-neon` | ROG Neon | dark | no |
| `rose-pine-dawn` | Rose Pine Dawn | light | no |
| `solarized-light` | Solarized Light | light | no |
| `tokyo-night` | Tokyo Night | dark | no |

You can list all built-in themes from the command line:

```sh
z13gui --list-themes
```

## Choosing a Built-in Theme

The easiest way to change themes is the theme picker button at the bottom of
the drawer. Click it to open a popover with all built-in themes. For themes
that support accent colors, a row of colored dots appears below the theme name.

Your selection is saved automatically to `~/.config/z13gui/config.toml`:

```toml
theme = "catppuccin-mocha"
accent = "blue"
```

You can also edit this file by hand. Changes take effect the next time z13gui
starts.

## Creating a Custom Theme

A custom theme is a TOML file with 7 color keys. Each value is a CSS hex color
string (`#rrggbb`).

**Quick start:**

```sh
mkdir -p ~/.config/z13gui
z13gui --print-theme > ~/.config/z13gui/theme.toml
```

This writes the default ROG Dark colors. Open the file in any text editor and
change the values to your liking.

**File format:**

```toml
# Accent color -- active buttons, slider fill, checked states
accent = "#cc0000"

# Drawer background
background = "#1a1a1a"

# Surface color -- button and row backgrounds
surface = "#2a2a2a"

# Alternate surface -- hover state background
surface_alt = "#333333"

# Primary text color
text = "#e0e0e0"

# Dim text color -- section labels, secondary descriptions
text_dim = "#888888"

# Border color -- window outline and separators
border = "#444444"
```

Comments (lines starting with `#`) and inline comments (` # ...` after a
value) are supported. Unknown keys are ignored. Missing keys fall back to the
ROG Dark defaults. Invalid hex values are skipped.

The `examples/themes/` directory in the source repository contains a `.toml`
file for every built-in theme. These are useful starting points for custom
themes.

Changes to `theme.toml` take effect the next time z13gui starts.

### Accent Variants

Custom themes can optionally define accent color variants. These appear as
colored dots in the theme picker, just like built-in Catppuccin themes. Add an
`[accents]` section after the color keys:

```toml
accent = "#5294e2"
background = "#1b2838"
surface = "#253449"
surface_alt = "#2e3f55"
text = "#d3dae3"
text_dim = "#7c8fa0"
border = "#3b5068"

[accents]
blue = "#5294e2"
teal = "#2eb398"
purple = "#9b59b6"
orange = "#e67e22"
```

Each line in the `[accents]` section is `id = "#hex"`. The ID is used as the
tooltip label and saved to `config.toml` when the user clicks a dot. Accent
variants only change the `accent` color -- all other colors stay the same.

The top-level `accent` key sets the default accent used when no variant is
selected. Your last accent selection is saved to `config.toml` and restored
on restart.

The `[accents]` section is entirely optional. If omitted, the theme works the
same as before -- a single fixed accent color with no picker dots.

## Color Reference

| Key | CSS Variable | What it controls |
|-----|-------------|------------------|
| `accent` | `@z13-accent` | Active buttons, slider fill, checked radio buttons, active tab highlight |
| `background` | `@z13-bg` | Drawer panel background |
| `surface` | `@z13-surface` | Button backgrounds, input row backgrounds, dropdown backgrounds |
| `surface_alt` | `@z13-surface-alt` | Hover state for buttons and rows |
| `text` | `@z13-text` | Primary text, labels, button text |
| `text_dim` | `@z13-text-dim` | Section headings (MODE, SPEED, etc.), secondary labels |
| `border` | `@z13-border` | Drawer border, separators, button outlines |

For light themes, `background` should be a light color and `text` should be
dark. For dark themes, the reverse. The `surface` color should be slightly
different from `background` so that buttons are visually distinct from the
drawer itself.

## Accent Colors

Some built-in themes offer accent color variants. All four Catppuccin themes
(Mocha, Macchiato, Frappe, and Latte) support the 14 official Catppuccin accent
colors.

You can set the accent in `config.toml`:

```toml
theme = "catppuccin-mocha"
accent = "sapphire"
```

**Available accent IDs:**

| ID | Mocha | Macchiato | Frappe | Latte |
|----|-------|-----------|--------|-------|
| `rosewater` | `#f5e0dc` | `#f4dbd6` | `#f2d5cf` | `#dc8a78` |
| `flamingo` | `#f2cdcd` | `#f0c6c6` | `#eebebe` | `#dd7878` |
| `pink` | `#f5c2e7` | `#f5bde6` | `#f4b8e4` | `#ea76cb` |
| `mauve` | `#cba6f7` | `#c6a0f6` | `#ca9ee6` | `#8839ef` |
| `red` | `#f38ba8` | `#ed8796` | `#e78284` | `#d20f39` |
| `maroon` | `#eba0ac` | `#ee99a0` | `#ea999c` | `#e64553` |
| `peach` | `#fab387` | `#f5a97f` | `#ef9f76` | `#fe640b` |
| `yellow` | `#f9e2af` | `#eed49f` | `#e5c890` | `#df8e1d` |
| `green` | `#a6e3a1` | `#a6da95` | `#a6d189` | `#40a02b` |
| `teal` | `#94e2d5` | `#8bd5ca` | `#81c8be` | `#179299` |
| `sky` | `#89dceb` | `#91d7e3` | `#99d1db` | `#04a5e5` |
| `sapphire` | `#74c7ec` | `#7dc4e4` | `#85c1dc` | `#209fb5` |
| `blue` | `#89b4fa` | `#8aadf4` | `#8caaee` | `#1e66f5` |
| `lavender` | `#b4befe` | `#b7bdf8` | `#babbf1` | `#7287fd` |

Accent colors only change the `accent` value in the theme. All other colors
(background, surface, text, etc.) remain the same.

Custom themes can also define their own accent variants using the `[accents]`
section in `theme.toml`. See [Accent Variants](#accent-variants) above.

## Full CSS Override

For complete control over the drawer appearance, you can provide a full CSS
stylesheet at `~/.config/z13gui/theme.css`. This replaces the built-in theme
CSS entirely.

The stylesheet should define all 7 `@define-color` variables:

```css
@define-color z13-accent      #cc0000;
@define-color z13-bg          #1a1a1a;
@define-color z13-surface     #2a2a2a;
@define-color z13-surface-alt #333333;
@define-color z13-text        #e0e0e0;
@define-color z13-text-dim    #888888;
@define-color z13-border      #444444;
```

You can then add any GTK4 CSS rules you want. This is intended for power users
who want to change fonts, spacing, border radius, or other structural
properties beyond what the 7-color system provides.

If `theme.toml` exists, it takes priority over `theme.css`. Delete `theme.toml`
to use a CSS override.

## Example

Here is a custom dark blue theme built from scratch:

```toml
# ~/.config/z13gui/theme.toml
# Custom dark blue theme

accent = "#5294e2"
background = "#1b2838"
surface = "#253449"
surface_alt = "#2e3f55"
text = "#d3dae3"
text_dim = "#7c8fa0"
border = "#3b5068"
```

Breaking it down:

- `accent` is a medium blue used for active buttons and checked states
- `background` is a very dark navy for the drawer panel
- `surface` is slightly lighter than the background so buttons stand out
- `surface_alt` is slightly lighter again for hover states
- `text` is a light gray-blue for readability against the dark background
- `text_dim` is a muted blue-gray for section labels
- `border` sits between surface and text brightness to provide subtle outlines

To use this theme, save it to `~/.config/z13gui/theme.toml` and restart
z13gui.
