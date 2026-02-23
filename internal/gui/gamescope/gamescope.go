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
	"fmt"
	"log/slog"
	"os"
	"strconv"
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

	outputWidth  int     // from realize; used in WrapContent for sizing
	outputHeight int     // from realize; used in WrapContent for margins
	scale        float64 // outputWidth / 1280, clamped [1.0, 3.0]
	panel        *gtk.Box
	onDismiss    func()
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

		// Store output dimensions for WrapContent (which runs after realize).
		// Compute a UI scale so the drawer occupies the same physical
		// screen fraction as KDE at 150% (~18.75% of screen width).
		// Z13GUI_SCALE env var overrides auto-detection.
		if monitor := display.MonitorAtSurface(surface); monitor != nil {
			geo := monitor.Geometry()
			b.outputWidth = geo.Width()
			b.outputHeight = geo.Height()
			if envScale := os.Getenv("Z13GUI_SCALE"); envScale != "" {
				if v, err := strconv.ParseFloat(envScale, 64); err == nil && v > 0 {
					b.scale = v
				}
			} else {
				b.scale = float64(geo.Width()) / 1707.0
			}
			if b.scale < 1.0 {
				b.scale = 1.0
			}
			if b.scale > 3.0 {
				b.scale = 3.0
			}
			b.appWin.SetDefaultSize(geo.Width(), geo.Height())
			slog.Info("gamescope: sized to monitor", "w", geo.Width(), "h", geo.Height(), "scale", b.scale)
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

	// Constrain drawer to scaled width.
	scaledWidth := int(float64(b.drawerWidth) * b.scale)
	if scaledWidth < b.drawerWidth {
		scaledWidth = b.drawerWidth
	}
	b.panel = gtk.NewBox(gtk.OrientationVertical, 0)
	panel := b.panel
	panel.SetSizeRequest(scaledWidth, -1)
	panel.SetHExpand(false)

	// 5% top/bottom margins (matches layer-shell backend).
	if b.outputHeight > 0 {
		margin := b.outputHeight / 20
		panel.SetMarginTop(margin)
		panel.SetMarginBottom(margin)
	}

	panel.Append(drawer)
	wrapper.Append(backdrop)
	wrapper.Append(panel)

	// Inject resolution-scaled CSS overrides for gamescope.
	if b.scale > 1.0 {
		css := gtk.NewCSSProvider()
		css.LoadFromString(b.scaledCSS())
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(), css,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION+1,
		)
	}

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

// scaledCSS returns CSS rules that override layout.css pixel values scaled by
// the gamescope resolution factor. Loaded at PRIORITY_APPLICATION+1 so it
// overrides the base layout.css. Only used when scale > 1.0.
func (b *Backend) scaledCSS() string {
	s := b.scale
	return fmt.Sprintf(`/* Gamescope resolution scaling (%.1fx) */
.drawer { font-size: %.0fpx; }
.drawer checkbutton { min-height: %.0fpx; padding: %.0fpx %.0fpx; border-radius: %.0fpx; }
.drawer .mode-grid checkbutton { min-height: %.0fpx; }
.tab-btn { min-height: %.0fpx; }
.drawer scale slider { min-width: %.0fpx; min-height: %.0fpx; }
.drawer scale value { margin-bottom: %.0fpx; }
.drawer-title { font-size: %.0fpx; letter-spacing: %.0fpx; }
.section-group { font-size: %.0fpx; letter-spacing: %.0fpx; margin-top: %.0fpx; }
.section-label { font-size: %.0fpx; letter-spacing: %.0fpx; margin-top: %.0fpx; margin-bottom: %.0fpx; }
.color-swatch { min-width: %.0fpx; min-height: %.0fpx; border-radius: %.0fpx; }
.color-preset { min-width: %.0fpx; min-height: %.0fpx; border-radius: %.0fpx; }
.bottom-bar button { min-width: %.0fpx; min-height: %.0fpx; padding: %.0fpx; border-radius: %.0fpx; }
.accent-label { font-size: %.0fpx; letter-spacing: %.0fpx; }
.accent-dot { min-width: %.0fpx; min-height: %.0fpx; }
.accent-dot-active { border-width: %.0fpx; }
.bottom-bar .toggle-label { font-size: %.0fpx; letter-spacing: %.1fpx; }
.bottom-bar switch { min-height: %.0fpx; min-width: %.0fpx; }
.bottom-bar switch slider { min-width: %.0fpx; min-height: %.0fpx; }
.view-back-btn { min-width: %.0fpx; min-height: %.0fpx; padding: %.0fpx; }`,
		s,
		14*s,                     // .drawer font-size
		48*s, 4*s, 10*s, 6*s,    // checkbutton
		52*s,                     // mode-grid checkbutton
		48*s,                     // tab-btn
		24*s, 24*s,               // scale slider
		6*s,                      // scale value margin
		11*s, 3*s,                // drawer-title
		13*s, 2*s, 2*s,           // section-group
		11*s, 1*s, 6*s, 2*s,     // section-label
		28*s, 28*s, 4*s,          // color-swatch
		28*s, 28*s, 4*s,          // color-preset
		32*s, 32*s, 4*s, 6*s,    // bottom-bar button
		9*s, 1*s,                 // accent-label
		14*s, 14*s,               // accent-dot
		2*s,                      // accent-dot-active border
		10*s, 0.5*s,              // toggle-label
		20*s, 36*s,               // bottom-bar switch
		18*s, 18*s,               // switch slider
		32*s, 32*s, 4*s,          // view-back-btn
	)
}
