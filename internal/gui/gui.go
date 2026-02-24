// Package gui implements the GTK4 overlay drawer for z13gui.
// It provides the main Window type that handles daemon state synchronization,
// GTK widget construction, and theming. Display-mode-specific concerns
// (layer-shell vs gamescope X11 overlay) are delegated to Backend implementations.
package gui

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/dahui/z13ctl/api"
	"github.com/dahui/z13gui/internal/gui/fonts"
	"github.com/dahui/z13gui/internal/gui/gamepad"
	"github.com/dahui/z13gui/internal/gui/gamescope"
	"github.com/dahui/z13gui/internal/gui/layershell"
	"github.com/dahui/z13gui/internal/theme"
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

const drawerWidth = 320 // drawer panel width in pixels

// Window is the overlay drawer. All methods must be called from the GTK main
// thread except subscribeLoop, which runs in a background goroutine.
type Window struct {
	win       *gtk.ApplicationWindow
	gtkWin    *gtk.Window // alias for backend calls
	backend   Backend     // display backend (layer-shell or gamescope)
	gamescope bool        // true when running under gamescope (X11 overlay mode)
	state     *api.State  // latest daemon state; nil until first successful fetch
	tab       string      // active device tab: "keyboard" or "lightbar"
	visible   bool        // true when the drawer is on-screen or animating in

	swatchProvider *gtk.CSSProvider // dynamic swatch background colors
	themeProvider  *gtk.CSSProvider // current theme; replaced on applyTheme()

	// Widget references for syncState.
	tabKB           *gtk.CheckButton
	tabLB           *gtk.CheckButton
	modeButtons     map[string]*gtk.CheckButton
	color1          *colorInput
	color2          *colorInput
	color1Box       *gtk.Box // COLOR 1 label + row — visibility toggled by syncModeVis
	color2Box       *gtk.Box // COLOR 2 label + row — visibility toggled by syncModeVis
	speedBox        *gtk.Box // SPEED label + row — visibility toggled by syncModeVis
	brightBox       *gtk.Box // BRIGHTNESS label + scale — hidden when mode is "off"
	speedBtns       map[string]*gtk.CheckButton
	brightScale     *gtk.Scale
	profileBtns     map[string]*gtk.CheckButton
	battScale       *gtk.Scale
	overdriveSwitch *gtk.Switch
	bootSoundSwitch *gtk.Switch

	syncing    bool        // true while syncState is updating widgets; suppresses sendApply
	applyTimer *time.Timer // debounce for continuous inputs (brightness, color wheel)

	// View switching (main/theme/color views).
	mainScroll         *gtk.ScrolledWindow // scrollable area in main drawer view
	themeScroll        *gtk.ScrolledWindow // scrollable area in theme picker view
	viewStack          *gtk.Stack          // switches between main/theme/color views
	editingColor       *colorInput         // which color the color-picker view is editing
	colorViewTitle     *gtk.Label          // "COLOR 1" or "COLOR 2" in color view header
	colorHue           *gtk.Scale          // H: 0-360
	colorSat           *gtk.Scale          // S: 0-100
	colorLit           *gtk.Scale          // L: 0-100
	colorPreview       *gtk.Box            // swatch preview in color view
	colorHexLabel      *gtk.Label          // hex display in color view
	colorSwatchProv    *gtk.CSSProvider    // color picker preview swatch CSS
	paletteBtn         *gtk.Button         // theme button in bottom bar
	themeBackBtn       *gtk.Button         // back button in theme view
	colorBackBtn       *gtk.Button         // back button in color picker view
	colorPickerPresets []*gtk.Button       // preset buttons in color picker view
	themeRadios        []*gtk.CheckButton  // collected during appendThemeChoices
	themeDots          [][]*gtk.Button     // accent dot buttons per theme

	// Custom theme state (set when theme.toml exists).
	isCustomTheme bool
	customColors  theme.Colors
	customAccents []theme.Accent

	// Gamepad focus navigation.
	gamepadReader     *gamepad.Reader
	focusItems        []focusItem // active view's navigable widgets (points to one of the lists below)
	focusIdx          int         // current position in focusItems
	gamepadActive     bool        // true when gamepad focus indicator is shown
	focusEditing      bool        // true when a slider is in edit mode
	editOriginalValue float64     // saved value for cancel on B
	mainFocusItems    []focusItem // focus grid for main drawer view
	themeFocusItems   []focusItem // focus grid for theme picker view
	colorFocusItems   []focusItem // focus grid for HSL color picker view
}

// New creates the overlay window and attaches it to app. Called from the
// GTK Activate signal.
func New(app *gtk.Application) *Window {
	w := &Window{
		tab:         "keyboard",
		gamescope:   os.Getenv("GAMESCOPE_WAYLAND_DISPLAY") != "",
		modeButtons: make(map[string]*gtk.CheckButton),
		speedBtns:   make(map[string]*gtk.CheckButton),
		profileBtns: make(map[string]*gtk.CheckButton),
	}

	w.win = gtk.NewApplicationWindow(app)
	w.win.AddCSSClass("z13-drawer-window")
	w.gtkWin = &w.win.Window

	// Select display backend.
	if w.gamescope {
		w.backend = gamescope.New(w.win, w.gtkWin, drawerWidth)
	} else {
		w.backend = layershell.New(w.win, w.gtkWin, drawerWidth)
	}

	w.backend.Configure(func() bool { return w.visible }, w.hide)

	// Register embedded Inter font before loading CSS so font-family resolves.
	fonts.Register()

	// Load CSS (layout + theme).
	w.loadCSS()

	// Swatch provider: separate from theme so we can update it per-color at runtime.
	w.swatchProvider = gtk.NewCSSProvider()
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), w.swatchProvider, gtk.STYLE_PROVIDER_PRIORITY_USER+10)

	// Build content and let the backend wrap it if needed.
	w.syncing = true
	w.win.SetChild(w.backend.WrapContent(w.buildContent()))
	w.syncing = false

	go w.subscribeLoop()

	// Gamepad input (disabled with Z13GUI_NO_GAMEPAD=1).
	if os.Getenv("Z13GUI_NO_GAMEPAD") == "" {
		w.gamepadReader = gamepad.New(
			w.handleGamepadAction,
			func() bool { return w.visible },
			func(f func()) { glib.IdleAdd(f) },
		)
		go w.gamepadReader.Run()
	}

	// Hide gamepad focus indicator on mouse movement.
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		if w.gamepadActive {
			w.hideGamepadFocus()
		}
	})
	w.gtkWin.AddController(motion)

	// Block arrow keys from reaching child widgets. GTK4 uses arrow keys
	// to navigate radio groups (auto-activating them) and adjust scales.
	// The overlay uses mouse/touch/gamepad — not keyboard navigation.
	keyBlock := gtk.NewEventControllerKey()
	keyBlock.SetPropagationPhase(gtk.PhaseCapture)
	keyBlock.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Up, gdk.KEY_Down, gdk.KEY_Left, gdk.KEY_Right:
			return true
		}
		return false
	})
	w.gtkWin.AddController(keyBlock)

	slog.Info("drawer initialized")
	return w
}

// Toggle shows or hides the drawer. Must be called from the GTK main thread.
func (w *Window) Toggle() {
	slog.Debug("toggle entered", "visible", w.visible)
	if w.visible {
		slog.Info("toggle", "action", "hide")
		w.hide()
	} else {
		slog.Info("toggle", "action", "show")
		w.show()
		fetchStart := time.Now()
		go func() {
			ok, state, err := api.SendGetState()
			slog.Debug("SendGetState returned", "ok", ok, "err", err, "elapsed", time.Since(fetchStart))
			if ok && err == nil {
				glib.IdleAdd(func() {
					slog.Debug("syncState dispatched", "totalElapsed", time.Since(fetchStart))
					w.state = state
					w.syncState()
				})
			} else if err != nil {
				slog.Warn("get state failed", "err", err)
			}
		}()
	}
}

// show delegates to the display backend.
func (w *Window) show() {
	slog.Debug("show called")
	w.visible = true
	if w.gamepadReader != nil {
		go w.gamepadReader.GrabAll()
	}
	w.backend.Show()
}

// hide delegates to the display backend. Resets to main view so the drawer
// always opens to the home screen.
func (w *Window) hide() {
	slog.Debug("hide called", "wasVisible", w.visible)
	w.visible = false
	if w.gamepadReader != nil {
		go w.gamepadReader.UngrabAll()
	}
	w.hideGamepadFocus()
	if w.viewStack != nil {
		w.viewStack.SetVisibleChildName("main")
		w.swapFocusList(w.mainFocusItems)
	}
	w.backend.Hide()
}

// handleGamepadAction processes a gamepad action on the GTK main thread.
// Navigation: D-pad moves between items. A activates buttons/switches or
// enters edit mode for sliders. In edit mode, left/right adjusts the value,
// A commits, B cancels.
func (w *Window) handleGamepadAction(action gamepad.Action) {
	if !w.gamepadActive {
		w.showGamepadFocus()
	}
	switch action {
	case gamepad.ActionUp:
		if w.focusEditing {
			w.exitEditMode(true)
		}
		w.moveVertical(-1)
	case gamepad.ActionDown:
		if w.focusEditing {
			w.exitEditMode(true)
		}
		w.moveVertical(1)
	case gamepad.ActionLeft:
		if w.focusEditing {
			w.adjustFocus(-1)
		} else {
			w.moveHorizontal(-1)
		}
	case gamepad.ActionRight:
		if w.focusEditing {
			w.adjustFocus(1)
		} else {
			w.moveHorizontal(1)
		}
	case gamepad.ActionAccept:
		if w.focusEditing {
			w.exitEditMode(true)
		} else {
			w.activateOrEdit()
		}
	case gamepad.ActionBack:
		if w.focusEditing {
			w.exitEditMode(false)
		} else if w.viewStack != nil && w.viewStack.VisibleChildName() != "main" {
			w.showMainView()
		} else {
			w.hide()
		}
	case gamepad.ActionBumpL:
		if w.focusEditing {
			w.exitEditMode(true)
		}
		w.jumpSection(-1)
	case gamepad.ActionBumpR:
		if w.focusEditing {
			w.exitEditMode(true)
		}
		w.jumpSection(1)
	}
}

// subscribeLoop runs in a background goroutine. It subscribes to daemon events
// and dispatches gui-toggle to the GTK main thread via IdleAdd. Reconnects
// with exponential backoff.
func (w *Window) subscribeLoop() {
	backoff := time.Second
	for {
		ch, cancel, err := api.Subscribe([]string{"gui-toggle"})
		if err != nil || ch == nil {
			slog.Info("daemon disconnected, retrying", "backoff", backoff)
			time.Sleep(backoff)
			if backoff < 3*time.Second {
				backoff *= 2
			}
			continue
		}
		slog.Info("daemon connected")
		backoff = time.Second
		for event := range ch {
			if event == "gui-toggle" {
				slog.Debug("gui-toggle received, dispatching")
				glib.TimeoutAdd(0, func() bool {
					w.Toggle()
					return false
				})
				glib.MainContextDefault().Wakeup()
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

	var loaded bool
	switch {
	case fileExists(tomlPath):
		data, err := os.ReadFile(tomlPath)
		if err != nil {
			slog.Warn("failed to read custom theme TOML, using default", "path", tomlPath, "err", err)
		} else {
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
			slog.Info("theme loaded", "source", "custom-toml", "path", tomlPath)
			loaded = true
		}
	case fileExists(cssPath):
		data, err := os.ReadFile(cssPath)
		if err != nil {
			slog.Warn("failed to read custom theme CSS, using default", "path", cssPath, "err", err)
		} else {
			w.themeProvider.LoadFromString(string(data))
			slog.Info("theme loaded", "source", "custom-css", "path", cssPath)
			loaded = true
		}
	}
	if !loaded {
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
		slog.Info("theme loaded", "source", "builtin", "theme", cfg.Theme, "accent", cfg.Accent)
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
	slog.Info("theme changed", "id", id, "accent", accentID)
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
