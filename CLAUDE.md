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

## Companion project: z13ctl

The `z13ctl` daemon (module `github.com/dahui/z13ctl`) is a sibling repo.
Its `api/` submodule (`github.com/dahui/z13ctl/api`) is published at tag `api/v1.1.7`
on GitHub.

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
  color.go                      colorInput widget + color picker view (math in internal/colorconv)
  errbar.go                     Error bar: reportError/clearError, the only user-facing error surface
  focus.go                      Focus widget adaptor (navigation logic in internal/focusgrid)
  layout.css                    Embedded structural CSS (touch targets, sizing) — PRIORITY_APPLICATION
  theme-default.css             Embedded theme template with @define-color placeholders — PRIORITY_USER
  theme-default.toml            Embedded default theme colors (rog-dark), used by --print-theme
internal/gui/fonts/
  font.go                       Embedded Inter font loading
internal/gui/layershell/
  layershell.go                 Layer-shell display backend (KDE/Wayland)
internal/gui/gamepad/
  gamepad.go                    evdev gamepad reader; device classification + EVIOCGRAB
  steam.go                      Steam PID discovery; drives the hidraw blocker
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
  theme.go                      Colors struct (8 tokens), 15 built-in themes, accent variants
  parse.go                      theme.toml parsing; starts from DefaultColors so missing keys
                                inherit defaults — this is what keeps old theme.toml files working
                                when a new color token is added
  css.go                        @define-color generation from a Colors value
  config.go                     Config persistence (selected theme/accent)
  *_test.go                     Theme parsing, CSS generation, and built-in completeness tests
internal/power/                 Limits value: TDP/fan bounds + rules (mirrors z13ctl internal/cli)
internal/focusgrid/             Gamepad focus navigation: row/col/section index math
internal/colorconv/             hex <-> HSL conversion and colour validation
internal/lighting/              RGB mode resolution, per-mode controls, defaults
internal/uiscale/               Gamescope UI scale factor (cannot live in the cgo package)
internal/startup/               CLI arg scanning + split-level slog handler
internal/togglegate/            Debounce helper for duplicate gui-toggle bursts
contrib/
  z13gui.service                systemd user service (EnvironmentFile for gamescope-session)
  z13gui.desktop                Desktop entry
```

## Key architectural decisions

- **Layer-shell** (KDE): `github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell`
  (NOT `gtklayershell` which is GTK3). pkg-config name: `gtk4-layer-shell-0`.
- **Anchor**: right + top + bottom edges. Top/bottom margins set to 5% of screen height
  on realize. The surface is pinned to its monitor via `SetMonitor` (helps wlroots
  compositors clip overflow; KWin does NOT clip, see conditional fade below).
- **Keyboard mode**: `LayerShellKeyboardModeOnDemand` — gets focus when visible.
- **Animation**: layer-shell right-margin animation (`gtk4layershell.SetMargin`).
  `margin=0` → on-screen; `margin=-320` → off-screen to the right.
  Avoids GTK Revealer which causes pixman errors and smearing artifacts in Wayland.
- **Window visibility**: window is kept `SetVisible(true)` at all times after creation.
  It's "hidden" by setting margin = -(width-1) (off-screen) and opacity = 0, not by
  destroying/hiding the surface. This prevents the ghost-surface artifact that KDE Plasma
  shows when remapping a surface. The 1px margin keeps the surface in KWin's composited
  output for damage tracking; opacity 0 makes the sliver invisible to the user.
- **Width**: `SetSizeRequest(320, -1)`. Height is natural (content-driven, scrolled).
- **Show/hide animation**: smoothstep easing via `AddTickCallback` (VSync-synced),
  with a shared `animGen` generation counter so a show cancels an in-flight hide
  (and vice versa). Two paths, chosen per Show/Hide by `hasRightNeighbor()`:
  - **No monitor to the right** → slide the right margin (`slideMargin`). Show sets
    opacity=1 then slides in; Hide slides out then sets opacity=0 (hides the 1px
    sliver on the primary's own right edge).
  - **A monitor to the right** → fade in place (`fadeOpacity`) at margin 0, fully on
    the primary, then park the transparent surface off-screen. A rightward slide
    would otherwise bleed onto that monitor because KWin doesn't clip layer-surface
    overflow to the assigned output. `Backend.margin`/`Backend.opacity` track current
    state; use `setMargin`/`setOpacity` to keep them in sync.
- **Single instance / activate guard**: `gtk.NewApplication("com.github.dahui.z13gui", 0)`
  registers the app on the session bus, so launching the binary a second time does not
  start a second process — GApplication forwards `activate` to the running instance and
  the new process exits. `main.go` therefore holds the `*gui.Window` and only calls
  `gui.New` on first activation; re-activation calls `Toggle()` instead. Without that
  guard each re-activation builds an entire second drawer (its own layer surface,
  subscribe loop, gamepad reader, telemetry poller) overlapping the first, and both stay
  live and interactive. One click on `contrib/z13gui.desktop` while the user service is
  running is enough to trigger it. Diagnostic: two `drawer initialized` log lines under
  a single PID means the guard is missing or broken.
- **State source of truth**: daemon is the source of truth. On show, `api.SendGetState()`
  is called and `syncState()` updates widgets. Widget signals are suppressed during sync
  via `Window.syncing bool`.
- **Error surface** (`errbar.go`): every daemon call reports failures through
  `w.reportError(op, err)` and clears on success with `clearError()`/`clearErrorAsync()`.
  The bar is appended to `outer` between `viewStack` and the bottom bar, so one instance
  covers all four views in both backends. **Never drop a daemon error into `slog` alone** —
  that is what made z13ctl issue #14 look like a dead button for weeks. `reportError` is
  safe from any goroutine (it marshals via `glib.IdleAdd`) and logs internally, so call
  sites should not also `slog.Warn`.
- **Daemon calls must not run on the GTK thread**: `api` commands carry a 10s deadline
  (`commandTimeout`, api v1.1.7), so an inline call freezes the drawer for up to 10s
  against a wedged daemon. Read widget values on the main thread, then do the socket
  round-trip in a goroutine — see `sendApply()` in `sync.go` for the pattern.
- **Decisions live outside `internal/gui`; widgets live inside it.** `internal/gui`
  needs CGO + GTK4 headers, so `make test` cannot even compile it — anything left in
  there is permanently unverifiable. Every rule, calculation or classification
  belongs in a pure package (`power`, `focusgrid`, `colorconv`, `lighting`,
  `uiscale`, `startup`, `togglegate`); the GTK files are thin adaptors that read
  widgets, call out, and apply the answer. Extracting logic this way has caught
  seven real bugs so far, none of which were found by reading the code.
  - `make test` derives its package list with
    `go list ./internal/... | grep -v /internal/gui`, so a new pure package is
    picked up automatically — nothing to remember.
  - **Never read or write a GTK widget from a goroutine.** GTK is not thread-safe;
    this is undefined behaviour, not a stale read. Snapshot widget values on the
    main thread into plain data, then do the socket call in the goroutine — see
    `readTdpRequest`/`tdpRequest.send` in `tdp.go` and `sendApply` in `sync.go`.
    Come back to the main thread with `glib.IdleAdd`.
- **Device limits are a value, not constants.** `power.Limits` holds the TDP range,
  fan floor, temperature axis and stock PPT table; `Window.limits` is initialised to
  `power.DefaultLimits()` (the Z13's values). **No TDP or fan bound may be hardcoded
  in `internal/gui`** — derive it from `w.limits` / `fc.limits()`, because z13ctl is
  being extended to devices with different envelopes. The design brief for the
  eventual daemon-served limits is in z13ctl's `.claude/plans/device-limits-api.md`;
  when it lands, only where `Window.limits` is assigned changes.
  - Presentation policy stays derived, not fixed: `BasicSliderMax()` is
    `TDPMaxSafe - 5`, not a literal 70, because 70 is meaningless on a device whose
    safe max is 54.
  - `Sanitized()` replaces zero fields with defaults, for the day a daemon older
    than the client omits one. `HighTDPMinPWM` is exempt — 0 legitimately means "no
    fan floor on this device".
  - `power.Curve` is a fixed `[8]` array. If a device ever needs a different point
    count it becomes a slice and the compile-time length guarantee is lost.
- **High-TDP fan floor**: while sustained PL1 exceeds 75W the daemon rejects any fan
  curve point below 204 PWM (80%) and refuses a fan reset outright. `fanFloorPWM()`
  derives this from applied daemon state (not slider position); `enforceConstraints`
  clamps drags to it, `fanCurveEditor.draw` renders the floor line, and `resetFanBtn` is
  desensitized with a tooltip pointing at Reset TDP. Both the threshold and the
  floor come from `w.limits`, not from literals.
- **Basic vs advanced TDP view**: basic mode is one slider applying a single value to
  all three limits, capped at 70W. `power.NeedsAdvanced` decides whether a state can be
  shown there; `syncCustomView` force-checks the Advanced box when it cannot. Without
  that the slider clamps, the label misreports the hardware, and a save sends the
  clamped value — silently lowering the user's power limit.
- **Subscribe loop**: background goroutine, exponential backoff reconnect, dispatches
  `Toggle()` onto the GTK main thread via `glib.TimeoutAdd(0, ...)` followed by
  `MainContextDefault().Wakeup()` (the wakeup is required — the loop may deliver an
  event while the main context is blocked).
- **Toggle debounce**: on some firmware revisions a single Armoury Crate press reaches
  the GUI as two `gui-toggle` events in the same instant, which cancel each other out
  (an open drawer gets hide → show and appears stuck open). `subscribeLoop` gates events
  through `togglegate.Accept(last, now, daemonToggleDebounce)` (leading edge: keep the
  first event of a burst, drop the rest; the window does not extend on suppression).
  **The window is 50ms and must stay well under ~120ms.** It is a duplicate filter, not
  an animation rate limiter: measured human tapping bottoms out near 129ms between
  presses, so a larger window discards deliberate input — the original 250ms swallowed
  38% of presses in a 96-event sample from real use. If a future change wants to rate
  limit the 200ms slide animation, do it in `Toggle()`/the backend, not here.
  The debounce timestamp is a **local variable** in `subscribeLoop`, not a `Window`
  field: every other piece of `Window` state is main-thread-owned, so keeping this one
  out of the struct makes it unreachable from the main thread and removes any chance of
  an unsynchronized read. Do not promote it to a field — reading it from `show()`/
  `hide()` would be a data race, and `internal/gui` is not covered by `go test -race`.
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
    - `.error-bar` / `.error-text` / `.error-dismiss` — error surface; colored via the
      `@z13-error` theme token, as is `.tdp-warning`
- **Profile selector**: buttons (`gtk.Button`), stored in
  `w.profileBtns map[string]*gtk.Button`. Not DropDown (popup broken in gamescope).
- **Focus-loss dismiss** (layer-shell): `EventControllerMotion` tracks `pointerInside`
  on the backend. On `notify::is-active` focus loss: if within 500ms of Show, ignored
  (compositor settle time for keyboard-mode transition). If pointer is inside, the drop
  is spurious (KDE Plasma briefly drops focus during keyboard-mode transitions) → ignored.
  If pointer is outside, user clicked elsewhere → dismiss after 200ms confirmation delay.
  Do NOT add a `focusedSinceShow` guard — it causes first-show dismiss regression on KDE
  where the compositor drops focus during keyboard-mode transition and never re-grants it.
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
- `api.SendOff(device string) (bool, error)` — turn off lighting for a device
- `api.SendProfileSet(profile string) (bool, error)`
- `api.SendBatteryLimitSet(limit int) (bool, error)`
- `api.SendPanelOverdriveSet(value int) (bool, error)` — 0 or 1
- `api.SendBootSoundSet(value int) (bool, error)` — 0 or 1
- `api.SendTdpSet(watts, pl1, pl2, pl3 string, force bool) (bool, error)` — set TDP.
  **`watts` (the `set` field) is mandatory even in advanced mode.** The daemon's
  `handleTDP` does `strconv.Atoi(req.Set)` before it looks at `pl1`/`pl2`/`pl3` and
  rejects the request with `TDP value must be an integer` if it is empty. `watts` is
  then used only as the default for any PL field left blank, so advanced mode passes
  PL1 as the base value (`SendTdpSet(pl1, pl1, pl2, pl3, force)` in `sendTdp()`).
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
    Enabled bool; Mode string; Color string; Color2 string
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
make test       # unit tests for the pure-Go packages (no GTK4 headers needed)
make lint       # golangci-lint run ./...
make clean      # rm z13gui
make snapshot   # goreleaser local build (no publish)
make release    # goreleaser build + publish
```

Requires at build time: `gtk4-layer-shell` C library (`pkg-config gtk4-layer-shell-0`).

`make test` derives its package list from `go list ./internal/...` minus
`internal/gui`, because `internal/gui` needs CGO and GTK4 headers while `go list`
only reads source. A new pure package is therefore tested automatically. `make race`
runs the same set under the race detector; `make cover` reports per-function
coverage.

CI (`.github/workflows/ci.yml`) runs tests + race on plain ubuntu and
build + gofmt + lint in the same Arch container as the release job. Before this
existed nothing ran tests on a push, which is how PR #10 merged tests that never
executed.

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
- 50ms duplicate filter on daemon `gui-toggle` events (`internal/togglegate`)
- Single-instance activate guard (re-activation toggles instead of building a second drawer)
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
