// Package gui implements the GTK4 layer-shell overlay drawer for z13gui.
// It provides the main Window type that handles show/hide animation,
// daemon state synchronization, and all GTK widget construction.
package gui

import (
	_ "embed"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/dahui/z13ctl/api"
	"github.com/dahui/z13gui/internal/theme"
	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed layout.css
var layoutCSS string

//go:embed theme-default.css
var defaultThemeCSS string

//go:embed theme-default.toml
var defaultThemeTOML string

const (
	drawerWidth  = 320                   // drawer panel width in pixels
	hiddenMargin = -(drawerWidth + 80)   // off-screen margin (extra buffer for wider surfaces)
	animDuration = 200 * time.Millisecond // slide animation duration
)

// Window is the overlay drawer. All methods must be called from the GTK main
// thread except subscribeLoop, which runs in a background goroutine.
type Window struct {
	win     *gtk.ApplicationWindow
	gtkWin  *gtk.Window // alias for layer-shell calls
	margin  int         // current right margin: 0=on-screen, hiddenMargin=off-screen
	animGen uint64      // incremented to cancel in-flight animations
	state   *api.State  // latest daemon state; nil until first successful fetch
	tab     string      // active device tab: "keyboard" or "lightbar"
	visible bool        // true when the drawer is on-screen or animating in

	swatchProvider *gtk.CSSProvider // dynamic swatch background colors
	themeProvider  *gtk.CSSProvider // current theme; replaced on applyTheme()

	// Widget references for syncState.
	tabKB       *gtk.CheckButton
	tabLB       *gtk.CheckButton
	modeButtons map[string]*gtk.CheckButton
	color1      *colorInput
	color2      *colorInput
	color1Box   *gtk.Box // COLOR 1 label + row — visibility toggled by syncModeVis
	color2Box   *gtk.Box // COLOR 2 label + row — visibility toggled by syncModeVis
	speedBox    *gtk.Box // SPEED label + row — visibility toggled by syncModeVis
	speedBtns   map[string]*gtk.CheckButton
	brightScale *gtk.Scale
	profileDrop *gtk.DropDown
	battScale   *gtk.Scale

	syncing    bool        // true while syncState is updating widgets; suppresses sendApply
	applyTimer *time.Timer // debounce for continuous inputs (brightness, color wheel)

	// Custom theme state (set when theme.toml exists).
	isCustomTheme bool
	customColors  theme.Colors
	customAccents []theme.Accent
}

// New creates the overlay window and attaches it to app. Called from the
// GTK Activate signal.
func New(app *gtk.Application) *Window {
	w := &Window{
		tab:         "keyboard",
		modeButtons: make(map[string]*gtk.CheckButton),
		speedBtns:   make(map[string]*gtk.CheckButton),
		margin:      hiddenMargin,
	}

	w.win = gtk.NewApplicationWindow(app)
	w.win.SetDecorated(false)
	w.win.AddCSSClass("z13-drawer-window")

	// Layer shell setup — must happen before the window is realized.
	w.gtkWin = &w.win.Window
	gtk4layershell.InitForWindow(w.gtkWin)
	gtk4layershell.SetLayer(w.gtkWin, gtk4layershell.LayerShellLayerOverlay)
	gtk4layershell.SetAnchor(w.gtkWin, gtk4layershell.LayerShellEdgeRight, true)
	gtk4layershell.SetAnchor(w.gtkWin, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(w.gtkWin, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetKeyboardMode(w.gtkWin, gtk4layershell.LayerShellKeyboardModeNone)
	// Start off-screen to the right; animation moves margin toward 0 to reveal.
	gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeRight, hiddenMargin)

	w.win.SetSizeRequest(drawerWidth, -1)

	// Load CSS (layout + theme).
	w.loadCSS()

	// Swatch provider: separate from theme so we can update it per-color at runtime.
	w.swatchProvider = gtk.NewCSSProvider()
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), w.swatchProvider, gtk.STYLE_PROVIDER_PRIORITY_USER+10)

	// Set content directly — no Revealer (causes smearing artifacts in Wayland Vulkan).
	w.syncing = true
	w.win.SetChild(w.buildContent())
	w.syncing = false

	// Hide when the window loses focus, but delay to allow child dialogs
	// (e.g. color picker) to become the active window first.
	w.win.Connect("notify::is-active", func() {
		if w.win.IsActive() || !w.visible {
			return
		}
		glib.TimeoutAdd(50, func() bool {
			if !w.visible || w.win.IsActive() {
				return false
			}
			active := w.win.Application().ActiveWindow()
			if active != nil && active.Object.Native() != w.gtkWin.Object.Native() {
				return false
			}
			w.hide()
			return false
		})
	})

	// Set top/bottom margins to 5% of screen height once the surface is realized.
	w.win.Connect("realize", func() {
		surface := w.win.Surface()
		if surface == nil {
			return
		}
		monitor := gdk.DisplayGetDefault().MonitorAtSurface(surface)
		if monitor == nil {
			return
		}
		geo := monitor.Geometry()
		margin := geo.Height() / 20
		gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeTop, margin)
		gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeBottom, margin)
	})

	// Keep the window surface alive at all times; "hidden" = margin off-screen.
	// This prevents KDE Plasma ghost-surface artifact on remap.
	w.win.SetVisible(true)

	go w.subscribeLoop()

	return w
}

// Toggle shows or hides the drawer. Must be called from the GTK main thread.
func (w *Window) Toggle() {
	if w.visible {
		w.hide()
	} else {
		w.show()
		go func() {
			if ok, state, err := api.SendGetState(); ok && err == nil {
				glib.IdleAdd(func() {
					w.state = state
					w.syncState()
				})
			}
		}()
	}
}

// show starts the slide-in animation using a smoothstep easing curve.
func (w *Window) show() {
	gtk4layershell.SetKeyboardMode(w.gtkWin, gtk4layershell.LayerShellKeyboardModeOnDemand)
	w.visible = true
	w.animGen++
	gen := w.animGen
	startMargin := w.margin
	startTime := time.Now()

	glib.TimeoutAdd(16, func() bool {
		if w.animGen != gen {
			return false
		}
		t := float64(time.Since(startTime)) / float64(animDuration)
		if t >= 1.0 {
			w.margin = 0
			gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeRight, 0)
			return false
		}
		t = t * t * (3 - 2*t) // smoothstep
		w.margin = startMargin + int(math.Round(float64(-startMargin)*t))
		gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeRight, w.margin)
		return true
	})
	w.win.Present()
}

// hide starts the slide-out animation.
func (w *Window) hide() {
	gtk4layershell.SetKeyboardMode(w.gtkWin, gtk4layershell.LayerShellKeyboardModeNone)
	w.visible = false
	w.animGen++
	gen := w.animGen
	startMargin := w.margin
	startTime := time.Now()

	glib.TimeoutAdd(16, func() bool {
		if w.animGen != gen {
			return false
		}
		t := float64(time.Since(startTime)) / float64(animDuration)
		if t >= 1.0 {
			w.margin = hiddenMargin
			gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeRight, hiddenMargin)
			return false
		}
		t = t * t * (3 - 2*t) // smoothstep
		w.margin = startMargin + int(math.Round(float64(hiddenMargin-startMargin)*t))
		gtk4layershell.SetMargin(w.gtkWin, gtk4layershell.LayerShellEdgeRight, w.margin)
		return true
	})
}

// subscribeLoop runs in a background goroutine. It subscribes to daemon events
// and dispatches gui-toggle to the GTK main thread via IdleAdd. Reconnects
// with exponential backoff.
func (w *Window) subscribeLoop() {
	backoff := time.Second
	for {
		ch, cancel, err := api.Subscribe([]string{"gui-toggle"})
		if err != nil || ch == nil {
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for event := range ch {
			if event == "gui-toggle" {
				glib.IdleAdd(func() bool {
					w.Toggle()
					return false
				})
			}
		}
		cancel()
	}
}

// loadCSS loads the layout CSS (always) then the user theme or the default theme.
// Priority chain (first match wins):
//  1. ~/.config/z13gui/theme.toml — custom color config (overrides everything)
//  2. ~/.config/z13gui/theme.css  — full CSS override (power users)
//  3. config.toml theme = "id"    — built-in theme selection
//  4. embedded "rog-dark"         — compiled-in default
func (w *Window) loadCSS() {
	display := gdk.DisplayGetDefault()

	layout := gtk.NewCSSProvider()
	layout.LoadFromString(layoutCSS)
	gtk.StyleContextAddProviderForDisplay(display, layout, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	w.themeProvider = gtk.NewCSSProvider()
	base := theme.XDGConfigHome()
	tomlPath := filepath.Join(base, "z13gui", "theme.toml")
	cssPath := filepath.Join(base, "z13gui", "theme.css")

	switch {
	case fileExists(tomlPath):
		data, _ := os.ReadFile(tomlPath)
		colors, accents := theme.ParseThemeTOMLFull(data)
		w.isCustomTheme = true
		w.customColors = colors
		w.customAccents = accents
		// Restore the user's last accent selection from config.toml.
		if len(accents) > 0 {
			cfg := theme.LoadAppConfig()
			if cfg.Accent != "" {
				for _, a := range accents {
					if a.ID == cfg.Accent {
						colors.Accent = a.Hex
						break
					}
				}
			}
		}
		w.themeProvider.LoadFromString(theme.BuildThemeCSS(colors, defaultThemeCSS))
	case fileExists(cssPath):
		data, _ := os.ReadFile(cssPath)
		w.themeProvider.LoadFromString(string(data))
	default:
		cfg := theme.LoadAppConfig()
		colors, ok := theme.BuiltinByID(cfg.Theme)
		if !ok {
			colors = theme.DefaultColors
		}
		if cfg.Accent != "" {
			if hex, ok := theme.BuiltinAccentHex(cfg.Theme, cfg.Accent); ok {
				colors.Accent = hex
			}
		}
		w.themeProvider.LoadFromString(theme.BuildThemeCSS(colors, defaultThemeCSS))
	}

	gtk.StyleContextAddProviderForDisplay(display, w.themeProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
}

// applyTheme hot-swaps the theme CSS provider and persists the selection to
// config.toml. accentID may be "" to use the theme's default accent.
// Must be called from the GTK main thread.
func (w *Window) applyTheme(id, accentID string) {
	display := gdk.DisplayGetDefault()
	if w.themeProvider != nil {
		gtk.StyleContextRemoveProviderForDisplay(display, w.themeProvider)
	}
	w.themeProvider = gtk.NewCSSProvider()
	colors, ok := theme.BuiltinByID(id)
	if !ok {
		colors = theme.DefaultColors
		id = "rog-dark"
	}
	if accentID != "" {
		if hex, ok := theme.BuiltinAccentHex(id, accentID); ok {
			colors.Accent = hex
		}
	}
	w.themeProvider.LoadFromString(theme.BuildThemeCSS(colors, defaultThemeCSS))
	gtk.StyleContextAddProviderForDisplay(display, w.themeProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
	theme.SaveAppConfig(theme.AppConfig{Theme: id, Accent: accentID})
}

// applyCustomAccent hot-swaps the accent color for a custom theme.toml theme.
// The accentID must match an entry in w.customAccents. The selection is saved
// to config.toml so it persists across restarts.
func (w *Window) applyCustomAccent(accentID string) {
	colors := w.customColors
	for _, a := range w.customAccents {
		if a.ID == accentID {
			colors.Accent = a.Hex
			break
		}
	}
	display := gdk.DisplayGetDefault()
	if w.themeProvider != nil {
		gtk.StyleContextRemoveProviderForDisplay(display, w.themeProvider)
	}
	w.themeProvider = gtk.NewCSSProvider()
	w.themeProvider.LoadFromString(theme.BuildThemeCSS(colors, defaultThemeCSS))
	gtk.StyleContextAddProviderForDisplay(display, w.themeProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
	theme.SaveAppConfig(theme.AppConfig{Accent: accentID})
}

// fileExists returns true if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DefaultThemeTOML returns the embedded default theme.toml content.
// Used by --print-theme to let users bootstrap a custom theme.
func DefaultThemeTOML() string {
	return defaultThemeTOML
}
