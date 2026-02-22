package gui

// color.go — color input widget: swatch + preset buttons + custom color chooser popover.

import (
	"fmt"
	"math"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// presetColors are the 8 quick-select colors shown as square buttons.
var presetColors = []string{
	"FF0000", // red
	"FF6600", // orange
	"FFFF00", // yellow
	"00FF00", // green
	"00FFFF", // cyan
	"0000FF", // blue
	"FF00FF", // magenta
	"FFFFFF", // white
}

// colorInput holds the current-color swatch, a row of preset buttons, a
// "Custom" MenuButton, and the ColorChooserWidget inside its popover.
type colorInput struct {
	row     *gtk.Box
	swatch  *gtk.Box                // current-color square (CSS ID driven)
	chooser *gtk.ColorChooserWidget //nolint:staticcheck // ColorDialog requires a portal; unusable on Wayland
	hex     string                  // current RRGGBB uppercase
}

// SetHex updates the stored color and pushes it to the chooser.
// Must only be called while w.syncing == true so the notify::rgba handler
// that fires from SetRGBA does not trigger a spurious sendApply.
func (ci *colorInput) SetHex(hex string) {
	ci.hex = strings.ToUpper(hex)
	ci.chooser.SetRGBA(hexToRGBA(ci.hex)) //nolint:staticcheck // see colorInput.chooser
}

// newColorInput creates a color input widget with swatch, preset buttons,
// and a Custom button that opens a ColorChooserWidget in a popover.
func (w *Window) newColorInput(initialHex, swatchName string) *colorInput {
	ci := &colorInput{hex: strings.ToUpper(initialHex)}

	// Current-color swatch (non-interactive colored square).
	ci.swatch = gtk.NewBox(gtk.OrientationHorizontal, 0)
	ci.swatch.SetName(swatchName)
	ci.swatch.AddCSSClass("color-swatch")

	// Color chooser widget lives inside a popover on the Custom button.
	// ColorChooserWidget is deprecated upstream but ColorDialog requires an
	// XDG portal that doesn't work correctly on Wayland compositors (KDE/Hyprland).
	ci.chooser = gtk.NewColorChooserWidget() //nolint:staticcheck // see block comment above
	ci.chooser.SetUseAlpha(false)            //nolint:staticcheck // see block comment above
	ci.chooser.SetSizeRequest(260, 320)
	ci.chooser.SetRGBA(hexToRGBA(ci.hex)) //nolint:staticcheck // see block comment above

	popover := gtk.NewPopover()
	popover.AddCSSClass("z13-popover")
	popover.SetChild(ci.chooser)
	popover.SetHasArrow(false)

	customBtn := gtk.NewMenuButton()
	customBtn.SetLabel("Custom")
	customBtn.SetPopover(popover)

	// Connect AFTER SetRGBA so the initial set doesn't fire the handler.
	ci.chooser.NotifyProperty("rgba", func() {
		if w.syncing {
			return
		}
		rgba := ci.chooser.RGBA() //nolint:staticcheck // see colorInput.chooser
		ci.hex = rgbaToHex(rgba)
		w.updateSwatches()
		w.queueApply()
	})

	// Row 1: preset buttons, each expanding equally to fill the row width.
	presetsRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	for _, hex := range presetColors {
		h := hex
		btn := gtk.NewButton()
		btn.AddCSSClass("color-preset")
		btn.SetHExpand(true)
		p := gtk.NewCSSProvider()
		p.LoadFromString(fmt.Sprintf("button.color-preset { background: #%s; }", h))
		btn.StyleContext().AddProvider(p, gtk.STYLE_PROVIDER_PRIORITY_USER+5) //nolint:staticcheck // per-widget dynamic color
		btn.ConnectClicked(func() {
			ci.hex = h
			w.updateSwatches()
			w.sendApply()
			w.syncing = true
			ci.chooser.SetRGBA(hexToRGBA(h)) //nolint:staticcheck // see colorInput.chooser
			w.syncing = false
		})
		presetsRow.Append(btn)
	}

	// Row 2: current-color swatch + Custom MenuButton.
	customBtn.SetHExpand(true)
	controlsRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	controlsRow.Append(ci.swatch)
	controlsRow.Append(customBtn)

	ci.row = gtk.NewBox(gtk.OrientationVertical, 4)
	ci.row.Append(presetsRow)
	ci.row.Append(controlsRow)
	return ci
}

// updateSwatches refreshes the shared CSS provider so both current-color
// swatches display the latest hex values.
func (w *Window) updateSwatches() {
	if w.swatchProvider == nil {
		return
	}
	c1 := "FF0000"
	if w.color1 != nil {
		c1 = w.color1.hex
	}
	c2 := "000000"
	if w.color2 != nil {
		c2 = w.color2.hex
	}
	w.swatchProvider.LoadFromString(fmt.Sprintf(
		"#color1-swatch { background-color: #%s; }\n#color2-swatch { background-color: #%s; }",
		c1, c2,
	))
}

// hexToRGBA converts a 6-digit uppercase hex string (e.g. "FF0000") to a gdk.RGBA.
func hexToRGBA(hex string) *gdk.RGBA {
	var ri, gi, bi uint8
	_, _ = fmt.Sscanf(hex, "%02X%02X%02X", &ri, &gi, &bi)
	rgba := gdk.NewRGBA(float32(ri)/255, float32(gi)/255, float32(bi)/255, 1.0)
	return &rgba
}

// rgbaToHex converts a gdk.RGBA to a 6-digit uppercase hex string (e.g. "FF0000").
// Returns "FF0000" (red) if rgba is nil.
func rgbaToHex(rgba *gdk.RGBA) string {
	if rgba == nil {
		return "FF0000"
	}
	r := int(math.Round(float64(rgba.Red()) * 255))
	g := int(math.Round(float64(rgba.Green()) * 255))
	b := int(math.Round(float64(rgba.Blue()) * 255))
	return fmt.Sprintf("%02X%02X%02X", r, g, b)
}
