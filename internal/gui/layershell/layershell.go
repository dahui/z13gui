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
	animDuration    = 200 * time.Millisecond // slide animation duration
	animGrace       = 300 * time.Millisecond // post-show grace before honouring dismiss
	focusLossDelay  = 200                    // ms; confirmation delay before auto-hide
	hiddenMarginPad = 80                     // extra px beyond drawerWidth for hidden margin
	marginFraction  = 20                     // screen height / N for 5% top/bottom margins
)

// Backend manages the layer-shell overlay drawer on Wayland compositors.
type Backend struct {
	appWin *gtk.ApplicationWindow
	gtkWin *gtk.Window

	drawerWidth   int
	hiddenMargin  int
	margin        int    // current right margin: 0=on-screen, hiddenMargin=off-screen
	animGen       uint64 // incremented to cancel in-flight animations
	animating     bool   // true during show/hide animation; suppresses focus-loss dismiss
	pointerInside bool   // true when pointer is over the drawer surface
}

// New creates a layer-shell backend. drawerWidth is the drawer panel width in pixels.
func New(appWin *gtk.ApplicationWindow, gtkWin *gtk.Window, drawerWidth int) *Backend {
	return &Backend{
		appWin:       appWin,
		gtkWin:       gtkWin,
		drawerWidth:  drawerWidth,
		hiddenMargin: -(drawerWidth + hiddenMarginPad),
		margin:       -(drawerWidth + hiddenMarginPad),
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
		margin := geo.Height() / marginFraction
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeTop, margin)
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeBottom, margin)
	})

	// Keep the window surface alive at all times; "hidden" = margin off-screen.
	// This prevents KDE Plasma ghost-surface artifact on remap.
	b.appWin.SetVisible(true)

	// Track pointer position to distinguish spurious KDE focus drops from
	// genuine click-outside. When KDE drops focus after rapid keyboard-mode
	// transitions, the pointer is still inside the drawer. When the user
	// clicks outside, the pointer is outside.
	motion := gtk.NewEventControllerMotion()
	motion.ConnectEnter(func(x, y float64) {
		b.pointerInside = true
	})
	motion.ConnectLeave(func() {
		b.pointerInside = false
	})
	b.gtkWin.AddController(motion)

	// Hide when the window loses focus, but only if the pointer is outside
	// the drawer (genuine click-outside). Ignore focus drops when the pointer
	// is inside (spurious KDE Plasma focus revocation).
	b.appWin.Connect("notify::is-active", func() {
		active := b.appWin.IsActive()
		vis := isVisible()
		slog.Debug("focus changed", "is-active", active, "visible", vis, "pointerInside", b.pointerInside)
		if active || !vis {
			return
		}
		if b.animating {
			slog.Debug("focus lost during animation, ignoring")
			return
		}
		if b.pointerInside {
			slog.Debug("focus lost but pointer inside drawer, ignoring spurious drop")
			return
		}
		slog.Debug("focus lost with pointer outside, dismissing after delay")
		glib.TimeoutAdd(focusLossDelay, func() bool {
			if !isVisible() || b.appWin.IsActive() || b.animating {
				slog.Debug("dismiss cancelled: hidden, refocused, or animating")
				return false
			}
			slog.Debug("dismiss confirmed: focus still lost")
			onDismiss()
			return false
		})
	})

	// Escape key dismiss (matches gamescope backend).
	key := gtk.NewEventControllerKey()
	key.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == 0xff1b { // GDK_KEY_Escape
			onDismiss()
			return true
		}
		return false
	})
	b.gtkWin.AddController(key)

	slog.Info("backend", "mode", "layer-shell")
}

// WrapContent returns the drawer as-is — layer-shell uses the drawer directly
// as the window child.
func (b *Backend) WrapContent(drawer gtk.Widgetter) gtk.Widgetter {
	return drawer
}

// Show starts the slide-in animation using a smoothstep easing curve.
func (b *Backend) Show() {
	slog.Debug("backend.Show", "startMargin", b.margin)
	gtk4layershell.SetKeyboardMode(b.gtkWin, gtk4layershell.LayerShellKeyboardModeOnDemand)

	var gen uint64
	gen = b.slideMargin(0, func() {
		b.appWin.Present()

		// Keep animating guard active for a grace period after show completes.
		// KDE Plasma's async keyboard-mode handling can send delayed focus
		// revocations after SetKeyboardMode(OnDemand); the grace period lets
		// the compositor settle before we honour dismiss events.
		glib.TimeoutAdd(uint(animGrace.Milliseconds()), func() bool {
			if b.animGen == gen {
				b.animating = false
				b.appWin.Present()
			}
			return false
		})
	})

	b.appWin.Present()
}

// Hide starts the slide-out animation.
func (b *Backend) Hide() {
	slog.Debug("backend.Hide", "startMargin", b.margin)

	// Clear GTK's internal focused-widget state before relinquishing keyboard
	// interactivity. Without this, GTK retains a stale focus reference that
	// prevents the compositor from cleanly re-granting focus on the next Show().
	b.gtkWin.SetFocus(nil)

	gtk4layershell.SetKeyboardMode(b.gtkWin, gtk4layershell.LayerShellKeyboardModeNone)
	b.pointerInside = false // clear stale state; surface stays mapped off-screen

	b.slideMargin(b.hiddenMargin, func() {
		b.animating = false
	})
}

// slideMargin animates b.margin from its current value to target over
// animDuration using smoothstep easing. onDone is called on the main thread
// when the animation completes (or nil to skip). Returns the animation
// generation for use in post-completion callbacks.
func (b *Backend) slideMargin(target int, onDone func()) uint64 {
	b.animGen++
	gen := b.animGen
	b.animating = true
	start := b.margin
	t0 := time.Now()

	glib.TimeoutAdd(16, func() bool {
		if b.animGen != gen {
			slog.Debug("anim cancelled", "gen", gen, "currentGen", b.animGen)
			return false
		}
		t := float64(time.Since(t0)) / float64(animDuration)
		if t >= 1.0 {
			b.margin = target
			gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, target)
			slog.Debug("anim complete", "gen", gen, "margin", target)
			if onDone != nil {
				onDone()
			}
			return false
		}
		t = t * t * (3 - 2*t) // smoothstep
		b.margin = start + int(math.Round(float64(target-start)*t))
		gtk4layershell.SetMargin(b.gtkWin, gtk4layershell.LayerShellEdgeRight, b.margin)
		return true
	})

	return gen
}
