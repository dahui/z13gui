// Package gamescope implements the X11 overlay display backend for gamescope
// (Steam Gaming Mode). It sets X11 atoms on the window so gamescope composites
// it as an external overlay above the running game.
package gamescope

/*
#cgo pkg-config: gtk4 x11
#cgo CFLAGS: -Wno-deprecated-declarations
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <stdint.h>
#include <stdlib.h>

// surface_get_xid returns the X11 Window ID from a realized GDK surface.
// gdk_x11_surface_get_xid is deprecated since GDK 4.18 but still works;
// gotk4's code generator skips it, so we call it via CGO.
static unsigned long surface_get_xid(void *gdk_surface) {
    return gdk_x11_surface_get_xid(GDK_SURFACE(gdk_surface));
}

// display_get_xdisplay returns the Xlib Display* from a GDK display.
static void *display_get_xdisplay(void *gdk_display) {
    return gdk_x11_display_get_xdisplay(GDK_DISPLAY(gdk_display));
}

// set_cardinal sets a 32-bit CARDINAL property on an X11 window.
static void set_cardinal(void *xdisplay, unsigned long xid,
                         const char *name, uint32_t value) {
    Display *dpy = (Display *)xdisplay;
    Atom atom = XInternAtom(dpy, name, False);
    XChangeProperty(dpy, xid, atom, XA_CARDINAL, 32,
                    PropModeReplace, (unsigned char *)&value, 1);
    XFlush(dpy);
}

// grab_keyboard grabs keyboard input to the specified window.
static int grab_keyboard(void *xdisplay, unsigned long xid) {
    return XGrabKeyboard((Display *)xdisplay, xid, True,
                         GrabModeAsync, GrabModeAsync, CurrentTime);
}

// grab_pointer grabs pointer input to the specified window.
static int grab_pointer(void *xdisplay, unsigned long xid) {
    return XGrabPointer((Display *)xdisplay, xid, True,
        ButtonPressMask | ButtonReleaseMask | PointerMotionMask,
        GrabModeAsync, GrabModeAsync, None, None, CurrentTime);
}

// ungrab_keyboard releases the keyboard grab.
static void ungrab_keyboard(void *xdisplay) {
    XUngrabKeyboard((Display *)xdisplay, CurrentTime);
    XFlush((Display *)xdisplay);
}

// ungrab_pointer releases the pointer grab.
static void ungrab_pointer(void *xdisplay) {
    XUngrabPointer((Display *)xdisplay, CurrentTime);
    XFlush((Display *)xdisplay);
}
*/
import "C"

import (
	"log/slog"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Backend manages the gamescope X11 overlay window.
type Backend struct {
	appWin      *gtk.ApplicationWindow
	gtkWin      *gtk.Window
	drawerWidth int

	xdisplay unsafe.Pointer
	xid      C.ulong
	ready    bool // true after realize extracts XID

	panel     *gtk.Box // drawer panel; margins set on realize
	onDismiss func()
}

// New creates a gamescope backend. drawerWidth is the drawer panel width in pixels.
func New(appWin *gtk.ApplicationWindow, gtkWin *gtk.Window, drawerWidth int) *Backend {
	return &Backend{
		appWin:      appWin,
		gtkWin:      gtkWin,
		drawerWidth: drawerWidth,
	}
}

// Configure sets up the fullscreen undecorated window and registers a realize
// handler that extracts the X11 window ID and sets the overlay atom.
// The window is kept mapped at all times; visibility is controlled by toggling
// _NET_WM_WINDOW_OPACITY (0 = hidden, 0xFFFFFFFF = visible). This avoids the
// Wayland surface lifecycle churn that SetVisible toggling causes in XWayland.
func (b *Backend) Configure(isVisible func() bool, onDismiss func()) {
	b.onDismiss = onDismiss
	b.appWin.SetDecorated(false)

	b.appWin.Connect("realize", func() {
		display := gdk.DisplayGetDefault()
		surfacer := b.appWin.Surface()
		if display == nil || surfacer == nil {
			slog.Warn("gamescope: could not get display or surface on realize")
			return
		}
		surface, ok := surfacer.(*gdk.Surface)
		if !ok || surface == nil {
			slog.Warn("gamescope: surface is not *gdk.Surface")
			return
		}
		b.xdisplay = C.display_get_xdisplay(unsafe.Pointer(display.Native())) //nolint:govet // GObject pointer is C-heap-allocated and pinned; uintptr→unsafe.Pointer is safe
		b.xid = C.surface_get_xid(unsafe.Pointer(surface.Native()))         //nolint:govet // GObject pointer is C-heap-allocated and pinned; uintptr→unsafe.Pointer is safe
		b.ready = true

		// Ensure window buffer matches output resolution so gamescope's
		// STEAM_OVERLAY rendering (no NoScale flag) doesn't scale/center.
		if monitor := display.MonitorAtSurface(surface); monitor != nil {
			geo := monitor.Geometry()
			b.appWin.SetDefaultSize(geo.Width(), geo.Height())
			// Match layer-shell's 5% top/bottom margins.
			if b.panel != nil {
				margin := geo.Height() / 20
				b.panel.SetMarginTop(margin)
				b.panel.SetMarginBottom(margin)
			}
			slog.Info("gamescope: sized to monitor", "w", geo.Width(), "h", geo.Height())
		}

		b.setAtom("STEAM_OVERLAY", true)
		b.setCardinal("_NET_WM_WINDOW_OPACITY", 0) // start hidden
		slog.Info("gamescope: overlay atom set", "xid", uint64(b.xid))
	})

	// Escape key dismiss.
	key := gtk.NewEventControllerKey()
	key.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == 0xff1b { // GDK_KEY_Escape
			onDismiss()
			return true
		}
		return false
	})
	b.gtkWin.AddController(key)

	b.appWin.Fullscreen()
	b.appWin.SetVisible(true) // keep mapped always; hidden via opacity=0
	slog.Info("backend", "mode", "gamescope")
}

// WrapContent wraps the drawer in a fullscreen container with a click-to-dismiss
// backdrop on the left and the drawer right-aligned.
func (b *Backend) WrapContent(drawer gtk.Widgetter) gtk.Widgetter {
	wrapper := gtk.NewBox(gtk.OrientationHorizontal, 0)
	wrapper.AddCSSClass("gs-wrapper")

	// Transparent click-to-dismiss backdrop.
	backdrop := gtk.NewBox(gtk.OrientationVertical, 0)
	backdrop.SetHExpand(true)
	backdrop.AddCSSClass("gs-backdrop")
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(nPress int, x, y float64) {
		slog.Debug("gamescope: backdrop clicked")
		if b.onDismiss != nil {
			b.onDismiss()
		}
	})
	backdrop.AddController(click)

	// Constrain drawer to its design width.
	b.panel = gtk.NewBox(gtk.OrientationVertical, 0)
	panel := b.panel
	panel.SetSizeRequest(b.drawerWidth, -1)
	panel.SetHExpand(false)
	panel.Append(drawer)

	wrapper.Append(backdrop)
	wrapper.Append(panel)
	return wrapper
}

// Show makes the overlay visible by setting full opacity and captures input
// via X11 grabs (primary) and STEAM_INPUT_FOCUS (secondary, for production).
func (b *Backend) Show() {
	if b.ready {
		b.setCardinal("_NET_WM_WINDOW_OPACITY", 0xFFFFFFFF)
		b.setAtom("STEAM_INPUT_FOCUS", true)
		C.grab_keyboard(b.xdisplay, b.xid)
		C.grab_pointer(b.xdisplay, b.xid)
		slog.Debug("gamescope: overlay shown")
	} else {
		slog.Warn("gamescope: Show() called but XID not ready")
	}
	b.appWin.Present()
}

// Hide releases input grabs and hides the overlay via zero opacity.
func (b *Backend) Hide() {
	if b.ready {
		C.ungrab_keyboard(b.xdisplay)
		C.ungrab_pointer(b.xdisplay)
		b.setAtom("STEAM_INPUT_FOCUS", false)
		b.setCardinal("_NET_WM_WINDOW_OPACITY", 0)
		slog.Debug("gamescope: overlay hidden")
	}
}

// setCardinal sets a 32-bit CARDINAL property on the overlay window.
func (b *Backend) setCardinal(name string, value uint32) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.set_cardinal(b.xdisplay, b.xid, cname, C.uint32_t(value))
}

// setAtom sets or clears a boolean CARDINAL atom (0 or 1).
func (b *Backend) setAtom(name string, on bool) {
	v := uint32(0)
	if on {
		v = 1
	}
	b.setCardinal(name, v)
}
