package gui

// errbar.go — the drawer's single user-facing error surface.
//
// Every daemon call in this package runs on a background goroutine and used to
// drop its error into slog and return, which made a failed operation look like a
// button that did nothing (see z13ctl issue #14, where "Save TDP" was silently
// rejected with "permission denied" for weeks). The bar lives outside the view
// stack, between it and the bottom bar, so one instance serves the main, custom,
// theme and color views in both the layer-shell and gamescope backends.
//
// Deliberately a plain Box + Label + Button: popovers are not composited under
// gamescope, and gtk.Revealer smears during the slide animation. Buttons use a
// CAPTURE-phase gesture internally, so touch works in gamescope without the
// addTouchActivate workaround needed for CheckButton and Switch.

import (
	"log/slog"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// errLabelMaxChars caps the error label's natural width request. The drawer is
// drawerWidth (320px) wide; at the 10px .error-text size this is comfortably
// inside the content area, so the bar never drives the drawer wider.
const errLabelMaxChars = 34

// buildErrorBar returns the hidden-by-default error strip. It is shown by
// reportError and hidden by clearError.
func (w *Window) buildErrorBar() *gtk.Box {
	bar := gtk.NewBox(gtk.OrientationHorizontal, 4)
	bar.AddCSSClass("error-bar")
	bar.SetVisible(false)

	w.errLabel = gtk.NewLabel("")
	w.errLabel.SetXAlign(0)
	w.errLabel.SetHExpand(true)
	w.errLabel.AddCSSClass("error-text")
	// Daemon messages embed sysfs paths, which are single unbreakable tokens.
	// A plain wrapping label reports the longest such token as its minimum width,
	// which widens the whole drawer to fit
	// "/sys/devices/platform/asus-nb-wmi/ppt_pl1_spl". WrapWordChar lets pango
	// break mid-token, and MaxWidthChars caps the natural width so the label
	// wraps into the drawer instead of stretching it.
	w.errLabel.SetWrap(true)
	w.errLabel.SetWrapMode(pango.WrapWordChar)
	w.errLabel.SetMaxWidthChars(errLabelMaxChars)
	bar.Append(w.errLabel)

	dismiss := gtk.NewButton()
	dismiss.SetIconName("window-close-symbolic")
	dismiss.SetTooltipText("Dismiss")
	dismiss.AddCSSClass("error-dismiss")
	dismiss.SetVAlign(gtk.AlignStart)
	dismiss.ConnectClicked(func() { w.clearError() })
	bar.Append(dismiss)

	w.errBar = bar
	return bar
}

// reportError shows err in the error bar and logs it. Safe to call from any
// goroutine: every daemon call in this package runs in its own goroutine, so the
// widget work is marshalled onto the GTK main thread.
//
// op should name the operation the way the user thinks of it ("Save TDP"), not
// the function that failed.
func (w *Window) reportError(op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("operation failed", "op", op, "err", err)
	msg := op + ": " + err.Error()
	glib.IdleAdd(func() {
		if w.errBar == nil || w.errLabel == nil {
			return
		}
		// Most recent error wins; the bar shows one message at a time.
		w.errLabel.SetLabel(msg)
		w.errBar.SetVisible(true)
	})
}

// clearError hides the error bar. Must be called from the GTK main thread.
// Called on each successful operation and from hide(), so a stale failure does
// not greet the user the next time the drawer opens.
func (w *Window) clearError() {
	if w.errBar == nil {
		return
	}
	w.errBar.SetVisible(false)
	if w.errLabel != nil {
		w.errLabel.SetLabel("")
	}
}

// clearErrorAsync is clearError for callers on a background goroutine — the
// success path of the same calls that use reportError.
func (w *Window) clearErrorAsync() {
	glib.IdleAdd(func() { w.clearError() })
}
