package gui

import (
	"context"
	"log/slog"
)

// filterHandler applies separate level thresholds for app logs vs GTK/GLib
// logs. gotk4's glib.init() routes all GLib/GTK messages through
// slog.Default(), adding a "glib_domain" attribute. This handler uses
// that attribute to distinguish GTK noise from application messages.
type filterHandler struct {
	inner    slog.Handler
	appLevel slog.Level // threshold for app messages (default: Info)
	gtkLevel slog.Level // threshold for GTK/GLib messages (default: Error)
}

// NewFilterHandler wraps inner with split-level filtering.
// appLevel controls the threshold for application log messages.
// gtkLevel controls the threshold for GTK/GLib messages (identified by the
// "glib_domain" attribute that gotk4 adds to every GLib log record).
func NewFilterHandler(inner slog.Handler, appLevel, gtkLevel slog.Level) *filterHandler {
	return &filterHandler{inner: inner, appLevel: appLevel, gtkLevel: gtkLevel}
}

func (h *filterHandler) Enabled(_ context.Context, level slog.Level) bool {
	// Must pass if either threshold is met — we can't distinguish source until Handle.
	return level >= h.appLevel || level >= h.gtkLevel
}

func (h *filterHandler) Handle(ctx context.Context, r slog.Record) error {
	isGTK := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "glib_domain" {
			isGTK = true
			return false
		}
		return true
	})
	if isGTK && r.Level < h.gtkLevel {
		return nil
	}
	if !isGTK && r.Level < h.appLevel {
		return nil
	}
	return h.inner.Handle(ctx, r)
}

func (h *filterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &filterHandler{inner: h.inner.WithAttrs(attrs), appLevel: h.appLevel, gtkLevel: h.gtkLevel}
}

func (h *filterHandler) WithGroup(name string) slog.Handler {
	return &filterHandler{inner: h.inner.WithGroup(name), appLevel: h.appLevel, gtkLevel: h.gtkLevel}
}
