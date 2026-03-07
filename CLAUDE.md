# z13gui — Project Context for Claude

## What this project is

`z13gui` is a GTK4 Wayland layer-shell overlay drawer for controlling the 2025 ASUS ROG
Flow Z13 via the `z13ctl` daemon. It slides in from the right edge of the screen when the
Armoury Crate button (KEY_PROG3) is pressed. The daemon broadcasts `gui-toggle` events over
a subscribe socket; this GUI listens for them.

It has two display backends:
- **Layer-shell** (KDE/Wayland): margin-based slide animation
- **Gamescope** (Steam Gaming Mode): X11 overlay via `STEAM_OVERLAY` atom

- Module: `github.com/dahui/z13gui`
- Binary: `z13gui`
- Repo: `/home/jeff/dev/z13gui`

## Companion project: z13ctl

The `z13ctl` daemon lives at `/home/jeff/dev/z13rgb` (module `github.com/dahui/z13ctl`).
Its `api/` submodule (`github.com/dahui/z13ctl/api`) is at `/home/jeff/dev/z13rgb/api/`
and is also published at tag `api/v1.1.3` on GitHub.

During local development, a `go.work` file in this repo (if present, gitignored) provides
the local override. In production the `go.mod` imports the published tag.

## Package layout

```
main.go                         GTK Application entry; ConnectActivate → gui.New(app)
                                Gamescope env detection + stale socket validation
Makefile                        build, install, lint, clean, snapshot, release
internal/gui/
  gui.go                        Window struct, backend selection, show/hide, subscribeLoop, theming
  backend.go                    Backend interface (Configure, WrapContent, Show, Hide)
  controls.go                   All GTK widget construction (drawer, views, bottom bar)
  tdp.go                        Custom profile view: TDP sliders, fan curve editor, undervolt, telemetry
  sync.go                       Daemon state sync and API send functions
  color.go                      colorInput struct, HSL conversion, color picker view logic
  log.go                        Split-level slog handler (app vs GTK noise filtering)
  layout.css                    Embedded structural CSS (touch targets, sizing) — PRIORITY_APPLICATION
  theme-default.css             Embedded theme template with @define-color placeholders — PRIORITY_USER
  theme-default.toml            Embedded default theme colors (rog-dark), used by --print-theme
internal/gui/layershell/
  layershell.go                 Layer-shell display backend (KDE/Wayland)
internal/gui/gamepad/hidblocker/
  hidblocker.go                 BPF LSM blocker: blocks hidraw reads for specific PIDs
  blocker.bpf.c                 BPF C program (SEC("lsm/file_permission"), returns -EAGAIN)
  gen.go                        bpf2go generate directive
  blocker_x86_bpfel.go          Generated Go bindings (committed)
  blocker_x86_bpfel.o           Generated BPF ELF object (committed)
  hidblocker_test.go            Tests (skip without root/BPF LSM)
  vmlinux.h                     Generated kernel BTF header (gitignored, machine-specific)
internal/gui/gamescope/
  gamescope.go                  Gamescope X11 overlay backend (Steam Gaming Mode)
internal/theme/
  theme.go                      Theme types, TOML parsing, CSS generation, config persistence
  builtins.go                   15 built-in themes (8 dark, 7 light) with accent variants
  theme_test.go                 Theme parsing and CSS generation tests
contrib/
  z13gui.service                systemd user service (EnvironmentFile for gamescope-session)
  z13gui.desktop                Desktop entry
```

## Key architectural decisions

- **Layer-shell** (KDE): `github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell`
  (NOT `gtklayershell` which is GTK3). pkg-config name: `gtk4-layer-shell-0`.
- **Anchor**: right edge only (`LayerShellEdgeRight`). No top/bottom anchor — compositor
  centers the window vertically at natural height.
- **Keyboard mode**: `LayerShellKeyboardModeOnDemand` — gets focus when visible.
- **Animation**: layer-shell right-margin animation (`gtk4layershell.SetMargin`).
  `margin=0` → on-screen; `margin=-320` → off-screen to the right.
  Avoids GTK Revealer which causes pixman errors and smearing artifacts in Wayland.
- **Window visibility**: window is kept `SetVisible(true)` at all times after creation.
  It's "hidden" by setting margin = -320 (off-screen), not by destroying/hiding the surface.
  This prevents the ghost-surface artifact that KDE Plasma shows when remapping a surface.
- **Width**: `SetSizeRequest(320, -1)`. Height is natural (content-driven, scrolled).
- **Slide animation**: smoothstep timer loop via `glib.TimeoutAdd(16, ...)`.
  Tracks current margin in `Window.margin` field; generation counter prevents overlapping.
- **State source of truth**: daemon is the source of truth. On show, `api.SendGetState()`
  is called and `syncState()` updates widgets. Widget signals are suppressed during sync
  via `Window.syncing bool`.
- **Subscribe loop**: background goroutine, exponential backoff reconnect, dispatches
  `Toggle()` via `glib.IdleAdd`.
- **CSS architecture**:
  - `layout.css` → `STYLE_PROVIDER_PRIORITY_APPLICATION` (structural, not overridable)
  - `theme-default.css` → `STYLE_PROVIDER_PRIORITY_USER` (colors, user-overridable)
  - No `hexpand: true` in CSS — use `widget.SetHExpand(true)` in Go instead.
  - No `box-shadow` on `.drawer` — it causes smearing outside the widget clip region
    during slide animations in Wayland Vulkan rendering.
  - No `AddMark()` on scales — scale marks inside an animated context cause GTK
    `GtkGizmo` allocation warnings and pixman errors.
  - CSS class hierarchy for text labels in custom view:
    - `.section-label` — section headers ("TDP", "UNDERVOLT", "FAN CURVE"): 11px, bold, letter-spaced, dim
    - `.scale-name` — slider name labels ("PL1 (SPL)", "CPU Curve Optimizer"): 10px, bold, no letter-spacing, dim
    - `.scale-value` — slider value readouts ("50 W", "CPU CO: -20"): 10px, normal weight, bright
- **Profile selector**: buttons (`gtk.Button`), stored in
  `w.profileBtns map[string]*gtk.Button`. Not DropDown (popup broken in gamescope).
- **Focus-loss dismiss** (layer-shell): `EventControllerMotion` tracks `pointerInside`
  on the backend. On `notify::is-active` focus loss: if pointer is inside, the drop is
  spurious (KDE Plasma briefly drops focus during keyboard-mode transitions) → ignored.
  If pointer is outside, user clicked elsewhere → dismiss after 200ms confirmation delay.
  Escape key also dismisses in both backends.
- **GTK_A11Y=none**: set in `main.go` and `contrib/z13gui.service`. Disables GTK4
  AT-SPI accessibility bridge, which sends D-Bus events on every widget state change.
  Under systemd (especially gamescope sessions), the AT-SPI bus may be unavailable,
  causing D-Bus timeouts that block GTK initialization.

## Gamescope backend (`internal/gui/gamescope/gamescope.go`)

The gamescope backend renders z13gui as an X11 overlay in Steam Gaming Mode.

- **Overlay type**: `STEAM_OVERLAY` atom (z-pos 3, interactive with input routing).
  NOT `GAMESCOPE_EXTERNAL_OVERLAY` (z-pos 2, display-only, no input).
- **Visibility**: opacity-based (`_NET_WM_WINDOW_OPACITY`). Window stays mapped always.
- **Input**: keyboard-only X11 grab (`XGrabKeyboard`) + `STEAM_INPUT_FOCUS` atom.
  `XGrabPointer` was removed because its core X11 event mask interferes with XI2
  touch delivery. STEAM_INPUT_FOCUS handles pointer/touch routing natively.
- **Scaling**: resolution-based CSS scaling (`outputWidth / 1707`). Reference 1707 = 2560/1.5
  (matches KDE 150% at Z13 native resolution). `Z13GUI_SCALE` env var overrides.
  GDK_SCALE CANNOT be used — causes double scaling (GTK + gamescope scaler).
- **Layout**: fullscreen window → horizontal box (backdrop + right-aligned panel).
  Panel has 5% top/bottom margins, scaled drawer width.
- **Popups don't work**: GTK4 popovers/dropdowns create separate X11 windows that
  gamescope doesn't composite. Solved via view switching (see below).

### View switching (gamescope only)

In both KDE and gamescope modes, `buildContent()` wraps content in a `gtk.Stack` with 4 pages:
- `"main"` — normal drawer (profiles, RGB, battery, etc.)
- `"custom"` — custom profile view (TDP, fan curve, undervolt, telemetry)
- `"theme"` — theme picker (radio buttons + accent dots, replaces popover in gamescope)
- `"color"` — HSL color picker (H/S/L sliders + presets + preview, replaces popover in gamescope)

Bottom bar stays visible across all views. `hide()` resets to "main".
In KDE mode, theme/color views use popovers instead of stack pages.

### Service environment

`contrib/z13gui.service` uses `EnvironmentFile=-%t/gamescope-environment` (optional).
`main.go` validates the gamescope Wayland socket exists before selecting the backend
to handle stale environment files after session switching.

## API usage (`github.com/dahui/z13ctl/api`)

Functions used:
- `api.SendGetState() (bool, *api.State, error)` — fetch full daemon state on show
- `api.Subscribe([]string{"gui-toggle"}) (<-chan string, func(), error)` — event stream
- `api.SendApply(device, color1, color2, mode, speed string, brightness int) (bool, error)`
- `api.SendProfileSet(profile string) (bool, error)`
- `api.SendBatteryLimitSet(limit int) (bool, error)`
- `api.SendPanelOverdriveSet(value int) (bool, error)` — 0 or 1
- `api.SendBootSoundSet(value int) (bool, error)` — 0 or 1
- `api.SendTdpSet(watts, pl1, pl2, pl3 string, force bool) (bool, error)` — set TDP
- `api.SendTdpReset() (bool, error)` — reset TDP to firmware defaults
- `api.SendFanCurveSet(curve string) (bool, error)` — set custom fan curve ("temp:pwm,..." format)
- `api.SendFanCurveReset() (bool, error)` — reset fan curves to auto
- `api.SendUndervoltSet(cpu string) (bool, error)` — set CPU Curve Optimizer offset
- `api.SendUndervoltReset() (bool, error)` — reset undervolt to stock (0)

Key types from `api`:
```go
type State struct {
    Lighting           LightingState
    Devices            map[string]LightingState  // keyed by "keyboard", "lightbar"
    Profile            string
    Battery            int
    BootSound          int  // 0 or 1
    PanelOverdrive     int  // 0 or 1
    TDP                *TDPState
    FanCurve           *FanCurveState
    Undervolt          *UndervoltState
    UndervoltAvailable bool  // true if ryzen_smu is loaded
    Temperature        int   // APU temp, degrees Celsius
    FanRPM             int   // fan1 speed in RPM
}
type LightingState struct {
    Mode string; Color string; Color2 string
    Speed string; Brightness int
}
type TDPState struct {
    PL1SPL int; PL2SPPT int; FPPT int
}
type FanCurveState struct {
    Mode   int              // 0=auto, 1=custom
    Points []FanCurvePoint  // 8 points
}
type FanCurvePoint struct {
    Temp int; PWM int
}
type UndervoltState struct {
    CPUCO  int   // all-core CPU Curve Optimizer offset (0 to -40)
    Active bool  // true when CO is applied to hardware
}
```

## Daemon socket

Path: `$XDG_RUNTIME_DIR/z13ctl/z13ctl.sock`

Daemon must be running for any `api.*` calls to succeed. If the daemon is not running,
`api.Subscribe` returns `nil, nil, nil` and `SendGetState` returns `false, nil, nil`.
The subscribe loop handles this with backoff retry.

## Build

```sh
make build      # CGO_ENABLED=1 go build -o z13gui .
sudo make install  # installs pre-built binary to /usr/local/bin/z13gui
make lint       # golangci-lint run ./...
make clean      # rm z13gui
make snapshot   # goreleaser local build (no publish)
make release    # goreleaser build + publish
```

Requires at build time: `gtk4-layer-shell` C library (`pkg-config gtk4-layer-shell-0`).

## Known GTK issues (do not re-introduce)

- **`hexpand: true` in CSS** — not a valid CSS property. Use `widget.SetHExpand(true)` in Go.
- **`scale.AddMark()`** — causes `GtkGizmo (slider) reported min width -2` warnings and
  pixman `Invalid rectangle` errors when the scale widget is in an animated context.
  Display-only values work fine with `SetDrawValue(true)`.
- **`gtk.Revealer` with `SlideLeft`** — causes smearing artifacts in Wayland Vulkan
  rendering because GTK's damage region doesn't properly clear the transparent areas left
  behind as the revealer collapses. Use layer-shell margin animation instead.
- **`SetSizeRequest` + Revealer** — keeping the window at fixed width while the Revealer
  collapses internally still leaves stale pixels; the compositor doesn't know the content
  region shrank.
- **`box-shadow` on animated containers** — shadow pixels extend outside the widget clip
  region and are not cleared each frame in Wayland Vulkan rendering, causing smearing.
- **GTK4 popovers in gamescope** — create separate override-redirect X11 windows that
  gamescope doesn't composite. Use `gtk.Stack` view switching instead (gamescope only).
- **GDK_SCALE in gamescope** — causes double scaling (GTK scales buffer, then gamescope
  scaler scales again). Use manual CSS scaling via `scaledCSS()` instead.
- **GtkDropDown in gamescope** — popup list is a separate X11 window. Use buttons or
  radio buttons instead. Profile selector uses `gtk.Button` with CSS `.active` class.
- **CheckButton/Switch touch in gamescope** — GTK4's CheckButton and Switch use an
  internal BUBBLE-phase GestureClick, which fails for touch input in gamescope/XWayland.
  Button widgets use CAPTURE phase and work fine. Workaround: `addTouchActivate()` in
  controls.go adds a touch-only (`SetTouchOnly(true)`) CAPTURE-phase GestureClick to each
  affected widget. Do not remove — without it, all CheckButtons and Switches are
  untappable via touchscreen in gamescope mode.

## Current status

Feature-complete for both KDE and gamescope modes:
- Margin-based slide animation (smoothstep, 200ms) — KDE
- Gamescope X11 overlay with opacity-based visibility + keyboard grab + STEAM_INPUT_FOCUS
- Touch activation workaround for gamescope (CAPTURE-phase GestureClick on CheckButton/Switch)
- Pointer-inside guard for KDE focus-loss handling (spurious drop vs genuine click-outside)
- GTK_A11Y=none for systemd AT-SPI timeout prevention
- RGB lighting controls (mode, color presets + custom chooser/HSL picker, speed, brightness)
- Profile switching via buttons (quiet/balanced/performance/custom)
- Custom profile view with:
  - TDP control: basic (single watt slider) and advanced (PL1/PL2/PL3) modes
  - Fan curve editor: 8-point Cairo graph with drag interaction, 35–105°C range
  - Undervolt: CPU Curve Optimizer slider (inside advanced TDP box, hidden when
    `ryzen_smu` unavailable). Slider shows 0 when not on custom profile.
    iGPU CO is not supported on Strix Halo.
  - Telemetry: APU temp + fan RPM in header and custom view, polled every 1s
  - Separate save/reset buttons for TDP, fans, and undervolt
- Battery charge limit slider
- Panel overdrive and boot sound toggles (footer switches)
- 15 built-in themes with accent variants + custom theme.toml support
- Gamescope view switching: theme picker view + HSL color picker view
- Resolution-based CSS scaling for gamescope (Z13GUI_SCALE override)
- Split-level logging (app=Info, GTK=Error; `-d` enables all Debug)
- goreleaser + GitHub Actions release pipeline
- systemd user service with optional gamescope-environment loading
