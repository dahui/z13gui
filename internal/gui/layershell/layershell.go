// Package layershell implements the Wayland layer-shell display backend.
// It handles layer-shell initialization, margin-based slide animation,
// and focus-loss auto-hide for compositors like KDE Plasma, Hyprland, and Sway.
package layershell

import (
	"log/slog"
	"math"
	"time"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	animDuration = 200 * time.Millisecond // slide animation duration
)

// Backend manages the layer-shell overlay drawer on Wayland compositors.
type Backend struct {
	appWin *gtk.ApplicationWindow
	gtkWin *gtk.Window

	drawerWidth  int
	hiddenMargin int
	margin       int    // current right margin: 0=on-screen, hiddenMargin=off-screen
	animGen      uint64 // incremented to cancel in-flight animations
}

// New creates a layer-shell backend. drawerWidth is the drawer panel width in pixels.
func New(appWin *gtk.ApplicationWindow, gtkWin *gtk.Window, drawerWidth int) *Backend {
	hidden := -(drawerWidth + 80) // extra buffer for wider surfaces
	return &Backend{
		appWin:       appWin,
		gtkWin:       gtkWin,
		drawerWidth:  drawerWidth,
		hiddenMargin: hidden,
		margin:       hidden,
	}
}

// Configure initializes layer-shell anchoring, margins, keyboard mode,
// the realize handler for monitor-based vertical margins, the keep-visible
// hack, and the focus-loss auto-hide handler.
func (b *Backend) Configure(isVisible func() bool, onDismiss func()) {
	b.appWin.SetDecorated(false)

	gtk4layershell.InitForWindow(b.gtkWin)
	gtk4layershell.SetLayer(b.gtkWin, gtk4layershell.LayerShellLayerOverlay)
	gtk4layershell.SetAnchor(b.gtkWin, gtk4layershell.LayerShellEdgeRight, true)
	gtk4layershell.SetAnchor(b.gtkWin, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(b.gtkWin, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetKeyboardMode(b.gtkWin, gtk4layershell.LayerShellKeyboardModeNone)
	gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, b.hiddenMargin)
	b.appWin.SetSizeRequest(b.drawerWidth, -1)

	// Set top/bottom margins to 5% of screen height once the surface is realized.
	b.appWin.Connect("realize", func() {
		surface := b.appWin.Surface()
		if surface == nil {
			return
		}
		monitor := gdk.DisplayGetDefault().MonitorAtSurface(surface)
		if monitor == nil {
			return
		}
		geo := monitor.Geometry()
		margin := geo.Height() / 20
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeTop, margin)
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeBottom, margin)
	})

	// Keep the window surface alive at all times; "hidden" = margin off-screen.
	// This prevents KDE Plasma ghost-surface artifact on remap.
	b.appWin.SetVisible(true)

	// Hide when the window loses focus, but delay to allow child popovers
	// (e.g. color picker) to become the active window first.
	b.appWin.Connect("notify::is-active", func() {
		if b.appWin.IsActive() || !isVisible() {
			return
		}
		glib.TimeoutAdd(50, func() bool {
			if !isVisible() || b.appWin.IsActive() {
				return false
			}
			active := b.appWin.Application().ActiveWindow()
			if active != nil && active.Object.Native() != b.gtkWin.Object.Native() {
				return false
			}
			onDismiss()
			return false
		})
	})

	slog.Info("backend", "mode", "layer-shell")
}

// WrapContent returns the drawer as-is — layer-shell uses the drawer directly
// as the window child.
func (b *Backend) WrapContent(drawer gtk.Widgetter) gtk.Widgetter {
	return drawer
}

// Show starts the slide-in animation using a smoothstep easing curve.
func (b *Backend) Show() {
	gtk4layershell.SetKeyboardMode(b.gtkWin, gtk4layershell.LayerShellKeyboardModeOnDemand)

	b.animGen++
	gen := b.animGen
	startMargin := b.margin
	startTime := time.Now()

	glib.TimeoutAdd(16, func() bool {
		if b.animGen != gen {
			return false
		}
		t := float64(time.Since(startTime)) / float64(animDuration)
		if t >= 1.0 {
			b.margin = 0
			gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, 0)
			return false
		}
		t = t * t * (3 - 2*t) // smoothstep
		b.margin = startMargin + int(math.Round(float64(-startMargin)*t))
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, b.margin)
		return true
	})
	b.appWin.Present()
}

// Hide starts the slide-out animation.
func (b *Backend) Hide() {
	gtk4layershell.SetKeyboardMode(b.gtkWin, gtk4layershell.LayerShellKeyboardModeNone)

	b.animGen++
	gen := b.animGen
	startMargin := b.margin
	startTime := time.Now()

	glib.TimeoutAdd(16, func() bool {
		if b.animGen != gen {
			return false
		}
		t := float64(time.Since(startTime)) / float64(animDuration)
		if t >= 1.0 {
			b.margin = b.hiddenMargin
			gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, b.hiddenMargin)
			return false
		}
		t = t * t * (3 - 2*t) // smoothstep
		b.margin = startMargin + int(math.Round(float64(b.hiddenMargin-startMargin)*t))
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, b.margin)
		return true
	})
}
