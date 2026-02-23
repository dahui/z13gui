package gui

// color.go — color input widget: swatch + preset buttons + custom color chooser popover.
// In gamescope mode, the custom button navigates to an HSL color picker view
// instead of opening a popover (GTK4 popovers create separate X11 windows that
// gamescope doesn't composite).

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
	label   string                  // display label ("COLOR 1" / "COLOR 2")
}

// SetHex updates the stored color and pushes it to the chooser (if present).
// Must only be called while w.syncing == true so the notify::rgba handler
// that fires from SetRGBA does not trigger a spurious sendApply.
func (ci *colorInput) SetHex(hex string) {
	ci.hex = strings.ToUpper(hex)
	if ci.chooser != nil {
		ci.chooser.SetRGBA(hexToRGBA(ci.hex)) //nolint:staticcheck // see colorInput.chooser
	}
}

// newColorInput creates a color input widget with swatch, preset buttons,
// and a Custom button that opens a ColorChooserWidget in a popover.
// In gamescope mode the Custom button navigates to the HSL color picker view.
func (w *Window) newColorInput(initialHex, swatchName, label string) *colorInput {
	ci := &colorInput{hex: strings.ToUpper(initialHex), label: label}

	// Current-color swatch (non-interactive colored square).
	ci.swatch = gtk.NewBox(gtk.OrientationHorizontal, 0)
	ci.swatch.SetName(swatchName)
	ci.swatch.AddCSSClass("color-swatch")

	if !w.gamescope {
		// Color chooser widget lives inside a popover on the Custom button.
		// ColorChooserWidget is deprecated upstream but ColorDialog requires an
		// XDG portal that doesn't work correctly on Wayland compositors (KDE/Hyprland).
		ci.chooser = gtk.NewColorChooserWidget() //nolint:staticcheck // see block comment above
		ci.chooser.SetUseAlpha(false)            //nolint:staticcheck // see block comment above
		ci.chooser.SetSizeRequest(260, 320)
		ci.chooser.SetRGBA(hexToRGBA(ci.hex)) //nolint:staticcheck // see block comment above
	}

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
			if ci.chooser != nil {
				w.syncing = true
				ci.chooser.SetRGBA(hexToRGBA(h)) //nolint:staticcheck // see colorInput.chooser
				w.syncing = false
			}
		})
		presetsRow.Append(btn)
	}

	ci.row = gtk.NewBox(gtk.OrientationVertical, 4)
	ci.row.Append(presetsRow)

	if w.gamescope {
		// Navigate to the HSL color picker view instead of opening a popover.
		editBtn := gtk.NewButton()
		editBtn.SetLabel("Custom")
		editBtn.SetHExpand(true)
		editBtn.ConnectClicked(func() { w.showColorView(ci) })
		controlsRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
		controlsRow.Append(ci.swatch)
		controlsRow.Append(editBtn)
		ci.row.Append(controlsRow)
	} else {
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

		// Row 2: current-color swatch + Custom MenuButton.
		customBtn.SetHExpand(true)
		controlsRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
		controlsRow.Append(ci.swatch)
		controlsRow.Append(customBtn)
		ci.row.Append(controlsRow)
	}

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

// showColorView navigates the gamescope view stack to the HSL color picker
// and initializes the sliders from the given colorInput's current hex.
func (w *Window) showColorView(ci *colorInput) {
	if w.viewStack == nil {
		return
	}
	w.editingColor = ci
	w.colorViewTitle.SetLabel(ci.label)
	h, s, l := hexToHSL(ci.hex)
	w.syncing = true
	w.colorHue.SetValue(h)
	w.colorSat.SetValue(s)
	w.colorLit.SetValue(l)
	w.syncing = false
	w.updateColorPreview()
	w.viewStack.SetVisibleChildName("color")
}

// onHSLChanged reads the current HSL slider values, converts to hex, and
// updates the editing color, swatches, and preview. Called by slider handlers.
func (w *Window) onHSLChanged() {
	if w.syncing || w.editingColor == nil {
		return
	}
	h := w.colorHue.Value()
	s := w.colorSat.Value()
	l := w.colorLit.Value()
	hex := hslToHex(h, s, l)
	w.editingColor.hex = hex
	w.updateSwatches()
	w.updateColorPreview()
	w.queueApply()
}

// colorPickerPresetClicked handles a preset button click in the color picker view.
func (w *Window) colorPickerPresetClicked(hex string) {
	if w.editingColor == nil {
		return
	}
	w.editingColor.hex = hex
	w.updateSwatches()
	w.sendApply()
	// Update HSL sliders to reflect the preset.
	h, s, l := hexToHSL(hex)
	w.syncing = true
	w.colorHue.SetValue(h)
	w.colorSat.SetValue(s)
	w.colorLit.SetValue(l)
	w.syncing = false
	w.updateColorPreview()
}

// updateColorPreview updates the color picker view's preview swatch and hex label.
func (w *Window) updateColorPreview() {
	if w.editingColor == nil || w.colorSwatchProv == nil {
		return
	}
	hex := w.editingColor.hex
	w.colorSwatchProv.LoadFromString(fmt.Sprintf(
		"#color-picker-preview { background-color: #%s; }", hex,
	))
	if w.colorHexLabel != nil {
		w.colorHexLabel.SetLabel("#" + hex)
	}
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

// hexToHSL converts a 6-digit hex string (e.g. "FF6600") to HSL components.
// Returns H in [0,360], S in [0,100], L in [0,100].
func hexToHSL(hex string) (h, s, l float64) {
	var ri, gi, bi uint8
	_, _ = fmt.Sscanf(hex, "%02X%02X%02X", &ri, &gi, &bi)
	r, g, b := float64(ri)/255, float64(gi)/255, float64(bi)/255

	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2

	if max == min {
		return 0, 0, l * 100
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	return h * 60, s * 100, l * 100
}

// hslToHex converts HSL components to a 6-digit hex string.
// H in [0,360], S in [0,100], L in [0,100].
func hslToHex(h, s, l float64) string {
	h, s, l = h/360, s/100, l/100
	if s == 0 {
		v := int(math.Round(l * 255))
		return fmt.Sprintf("%02X%02X%02X", v, v, v)
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r := hueToRGB(p, q, h+1.0/3.0)
	g := hueToRGB(p, q, h)
	b := hueToRGB(p, q, h-1.0/3.0)
	return fmt.Sprintf("%02X%02X%02X",
		int(math.Round(r*255)),
		int(math.Round(g*255)),
		int(math.Round(b*255)),
	)
}

// hueToRGB is a helper for HSL→RGB conversion.
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}
