package gui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// Backend abstracts display-mode-specific window management.
// Implementations: layershell.Backend (Wayland compositors) and
// gamescope.Backend (X11 overlay for Steam Gaming Mode).
type Backend interface {
	// Configure sets up the window for this display mode. Must be called
	// before the window is realized.
	// isVisible reports whether the drawer is currently on-screen.
	// onDismiss is called when the backend wants to hide the drawer
	// (focus loss in layer-shell, backdrop click in gamescope).
	Configure(isVisible func() bool, onDismiss func())

	// WrapContent optionally wraps the drawer widget for this display mode.
	// Layer-shell returns it as-is; gamescope wraps it in a fullscreen
	// container with a click-to-dismiss backdrop.
	WrapContent(drawer gtk.Widgetter) gtk.Widgetter

	// Show makes the drawer visible (animation, atom toggle, etc).
	Show()

	// Hide hides the drawer (animation, atom toggle, etc).
	Hide()
}
