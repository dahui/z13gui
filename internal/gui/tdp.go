package gui

// tdp.go — Custom profile view: TDP sliders, fan curve editor, telemetry.

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/dahui/z13ctl/api"
	"github.com/dahui/z13gui/internal/power"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// fanCurveEditor renders and handles interaction for the 8-point fan curve.
// The curve model and its constraint rules live in internal/power; this type
// owns only the drawing and pointer handling.
type fanCurveEditor struct {
	area     *gtk.DrawingArea
	points   power.Curve // temp/PWM bounds come from Window.limits
	dragging int         // point index being dragged, -1 if none
	hovered  int         // point index under cursor, -1 if none
	w        *Window     // parent for theme colors + telemetry

	// Chart area within the DrawingArea (set during draw).
	chartX, chartY, chartW, chartH float64
}

// curveString returns the curve in "temp:pwm,temp:pwm,..." format for the API.
func (fc *fanCurveEditor) curveString() string { return fc.points.String() }

// limits returns the device envelope driving the editor's axes and clamping,
// falling back to the defaults when the editor has no parent window.
func (fc *fanCurveEditor) limits() power.Limits {
	if fc.w == nil {
		return power.DefaultLimits()
	}
	return fc.w.limits
}

// tempRange is the editor's x axis, in Celsius.
func (fc *fanCurveEditor) tempRange() (lo, hi int) {
	l := fc.limits()
	return l.TempMin, l.TempMax
}

// fanFloorPWM returns the minimum fan PWM the daemon will currently accept.
//
// Derived from the daemon's applied state rather than the slider position: the
// daemon validates against hardware, and a slider the user has moved but not
// saved has not been applied. Must be called from the GTK main thread.
func (w *Window) fanFloorPWM() int {
	if w.state == nil || w.state.TDP == nil {
		return power.PWMMin
	}
	return w.limits.FanFloorPWM(w.state.TDP.PL1SPL)
}

// minPWM is the editor's view of fanFloorPWM, safe when the editor has no parent.
func (fc *fanCurveEditor) minPWM() int {
	if fc.w == nil {
		return power.PWMMin
	}
	return fc.w.fanFloorPWM()
}

// enforceConstraints repairs the curve after point idx moved. The rules live in
// internal/power, where they are unit tested.
func (fc *fanCurveEditor) enforceConstraints(idx int) {
	fc.limits().EnforceCurve(&fc.points, idx, fc.minPWM())
}

// Coordinate mapping.
func (fc *fanCurveEditor) tempToX(temp int) float64 {
	lo, hi := fc.tempRange()
	return fc.chartX + (float64(temp-lo)/float64(hi-lo))*fc.chartW
}
func (fc *fanCurveEditor) pwmToY(pwm int) float64 {
	return fc.chartY + fc.chartH - (float64(pwm)/float64(power.PWMMax))*fc.chartH // inverted
}
func (fc *fanCurveEditor) xToTemp(x float64) int {
	lo, hi := fc.tempRange()
	t := lo + int(math.Round((x-fc.chartX)/fc.chartW*float64(hi-lo)))
	if t < lo {
		t = lo
	}
	if t > hi {
		t = hi
	}
	return t
}
func (fc *fanCurveEditor) yToPWM(y float64) int {
	p := int(math.Round((fc.chartY + fc.chartH - y) / fc.chartH * float64(power.PWMMax)))
	if p < power.PWMMin {
		p = power.PWMMin
	}
	if p > power.PWMMax {
		p = power.PWMMax
	}
	return p
}

// hitTest returns the index of the point nearest to (x,y) within tolerance, or -1.
func (fc *fanCurveEditor) hitTest(x, y float64) int {
	const tolerance = 20.0
	best := -1
	bestDist := tolerance * tolerance
	for i, p := range fc.points {
		px := fc.tempToX(p.Temp)
		py := fc.pwmToY(p.PWM)
		dx := x - px
		dy := y - py
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// draw renders the fan curve chart.
func (fc *fanCurveEditor) draw(cr *cairo.Context, width, height int) {
	w := float64(width)
	h := float64(height)

	// Chart margins.
	const leftMargin = 36.0
	const bottomMargin = 20.0
	const topMargin = 8.0
	const rightMargin = 8.0
	fc.chartX = leftMargin
	fc.chartY = topMargin
	fc.chartW = w - leftMargin - rightMargin
	fc.chartH = h - topMargin - bottomMargin

	// Background.
	cr.SetSourceRGBA(0, 0, 0, 0) // transparent — CSS handles bg
	cr.Paint()

	// Grid lines.
	cr.SetSourceRGBA(0.4, 0.4, 0.4, 0.3)
	cr.SetLineWidth(0.5)
	// Horizontal: 0%, 25%, 50%, 75%, 100%.
	for _, pct := range []float64{0, 25, 50, 75, 100} {
		y := fc.pwmToY(int(pct / 100.0 * power.PWMMax))
		cr.MoveTo(fc.chartX, y)
		cr.LineTo(fc.chartX+fc.chartW, y)
	}
	// Vertical: every 10°C across the device's range.
	tLo, tHi := fc.tempRange()
	for temp := tLo; temp <= tHi; temp += 10 {
		x := fc.tempToX(temp)
		cr.MoveTo(x, fc.chartY)
		cr.LineTo(x, fc.chartY+fc.chartH)
	}
	cr.Stroke()

	// Axis labels.
	cr.SetSourceRGBA(0.6, 0.6, 0.6, 1)
	cr.SetFontSize(9)
	// Y-axis labels.
	for _, pct := range []int{0, 25, 50, 75, 100} {
		y := fc.pwmToY(int(float64(pct) / 100.0 * power.PWMMax))
		cr.MoveTo(2, y+3)
		cr.ShowText(fmt.Sprintf("%d%%", pct))
	}
	// X-axis labels.
	for temp := tLo + 5; temp <= tHi-5; temp += 20 {
		x := fc.tempToX(temp)
		cr.MoveTo(x-8, fc.chartY+fc.chartH+14)
		cr.ShowText(fmt.Sprintf("%d°", temp))
	}

	// High-TDP fan floor. While sustained TDP is above the safe max the daemon
	// rejects any point below this line, and enforceConstraints holds drags at or
	// above it — drawing it explains why the points will not go lower.
	if floor := fc.minPWM(); floor > 0 {
		fy := fc.pwmToY(floor)
		cr.SetSourceRGBA(1, 0.27, 0.27, 0.9) // matches .tdp-warning red
		cr.SetLineWidth(1.5)
		cr.SetDash([]float64{6, 3}, 0)
		cr.MoveTo(fc.chartX, fy)
		cr.LineTo(fc.chartX+fc.chartW, fy)
		cr.Stroke()
		cr.SetDash(nil, 0)
		cr.SetFontSize(9)
		cr.MoveTo(fc.chartX+4, fy-4)
		cr.ShowText(fmt.Sprintf("%d%% min (TDP > %dW)", floor*100/power.PWMMax, fc.limits().TDPMaxSafe))
	}

	// Current APU temperature indicator line.
	if fc.w != nil && fc.w.state != nil && fc.w.state.Temperature > 0 {
		apuTemp := fc.w.state.Temperature
		if apuTemp >= tLo && apuTemp <= tHi {
			tx := fc.tempToX(apuTemp)
			cr.SetSourceRGBA(1, 1, 1, 0.4)
			cr.SetLineWidth(1)
			cr.SetDash([]float64{4, 3}, 0)
			cr.MoveTo(tx, fc.chartY)
			cr.LineTo(tx, fc.chartY+fc.chartH)
			cr.Stroke()
			cr.SetDash(nil, 0)
		}
	}

	// Filled area under curve.
	cr.SetSourceRGBA(0.8, 0.1, 0.1, 0.15) // accent-ish, semi-transparent
	cr.MoveTo(fc.tempToX(fc.points[0].Temp), fc.pwmToY(0))
	for _, p := range fc.points {
		cr.LineTo(fc.tempToX(p.Temp), fc.pwmToY(p.PWM))
	}
	cr.LineTo(fc.tempToX(fc.points[7].Temp), fc.pwmToY(0))
	cr.ClosePath()
	cr.Fill()

	// Line connecting points.
	cr.SetSourceRGBA(0.8, 0.1, 0.1, 1) // accent color
	cr.SetLineWidth(2)
	for i, p := range fc.points {
		x := fc.tempToX(p.Temp)
		y := fc.pwmToY(p.PWM)
		if i == 0 {
			cr.MoveTo(x, y)
		} else {
			cr.LineTo(x, y)
		}
	}
	cr.Stroke()

	// Point circles.
	for i, p := range fc.points {
		x := fc.tempToX(p.Temp)
		y := fc.pwmToY(p.PWM)
		radius := 6.0
		if i == fc.dragging || i == fc.hovered {
			radius = 8.0
			// Outer ring.
			cr.SetSourceRGBA(1, 1, 1, 0.6)
			cr.Arc(x, y, radius+2, 0, 2*math.Pi)
			cr.Stroke()
		}
		cr.SetSourceRGBA(0.8, 0.1, 0.1, 1)
		cr.Arc(x, y, radius, 0, 2*math.Pi)
		cr.Fill()
	}
}

// newFanCurveEditor creates the DrawingArea and sets up input handling.
func (w *Window) newFanCurveEditor() *fanCurveEditor {
	fc := &fanCurveEditor{
		dragging: -1,
		hovered:  -1,
		w:        w,
		points:   w.limits.DefaultCurve(),
	}

	fc.area = gtk.NewDrawingArea()
	fc.area.AddCSSClass("fan-curve-area")
	fc.area.SetSizeRequest(-1, 240)
	fc.area.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		fc.draw(cr, width, height)
	})

	// Drag gesture for point dragging (CAPTURE phase for gamescope touch).
	drag := gtk.NewGestureDrag()
	drag.SetPropagationPhase(gtk.PhaseCapture)

	var startX, startY float64

	drag.ConnectDragBegin(func(x, y float64) {
		idx := fc.hitTest(x, y)
		if idx < 0 {
			drag.SetState(gtk.EventSequenceDenied)
			return
		}
		fc.dragging = idx
		startX, startY = x, y
		fc.area.QueueDraw()
	})

	drag.ConnectDragUpdate(func(offsetX, offsetY float64) {
		if fc.dragging < 0 {
			return
		}
		x := startX + offsetX
		y := startY + offsetY
		fc.points[fc.dragging].Temp = fc.xToTemp(x)
		fc.points[fc.dragging].PWM = fc.yToPWM(y)
		fc.enforceConstraints(fc.dragging)
		fc.area.QueueDraw()
	})

	drag.ConnectDragEnd(func(_, _ float64) {
		fc.dragging = -1
		fc.area.QueueDraw()
	})

	fc.area.AddController(drag)

	// Hover tracking (mouse only).
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		if fc.dragging >= 0 {
			return
		}
		prev := fc.hovered
		fc.hovered = fc.hitTest(x, y)
		if fc.hovered != prev {
			fc.area.QueueDraw()
		}
	})
	motion.ConnectLeave(func() {
		if fc.hovered >= 0 {
			fc.hovered = -1
			fc.area.QueueDraw()
		}
	})
	fc.area.AddController(motion)

	return fc
}

// buildCustomView builds the custom TDP/fan curve view.
func (w *Window) buildCustomView() *gtk.Box {
	view := gtk.NewBox(gtk.OrientationVertical, 0)

	// Header: back button + title.
	w.customBackBtn = gtk.NewButton()
	w.customBackBtn.SetIconName("go-previous-symbolic")
	w.customBackBtn.AddCSSClass("view-back-btn")
	w.customBackBtn.ConnectClicked(func() { w.showMainView() })

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.SetMarginTop(10)
	header.SetMarginBottom(6)
	header.SetMarginStart(14)
	header.Append(w.customBackBtn)
	lbl := gtk.NewLabel("Custom Profile")
	lbl.SetHAlign(gtk.AlignStart)
	lbl.AddCSSClass("drawer-title")
	header.Append(lbl)
	view.Append(header)

	content := gtk.NewBox(gtk.OrientationVertical, 8)
	content.SetMarginTop(4)
	content.SetMarginBottom(12)
	content.SetMarginStart(12)
	content.SetMarginEnd(12)

	// --- TELEMETRY ---
	content.Append(sectionLabel("TELEMETRY"))
	telRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	w.telemetryTempLabel = gtk.NewLabel("APU: --°C")
	w.telemetryTempLabel.SetHAlign(gtk.AlignStart)
	w.telemetryTempLabel.AddCSSClass("section-label")
	w.telemetryFanLabel = gtk.NewLabel("Fan: -- RPM")
	w.telemetryFanLabel.SetHAlign(gtk.AlignEnd)
	w.telemetryFanLabel.SetHExpand(true)
	w.telemetryFanLabel.AddCSSClass("section-label")
	telRow.Append(w.telemetryTempLabel)
	telRow.Append(w.telemetryFanLabel)
	content.Append(telRow)

	// --- TDP ---
	content.Append(sectionLabel("TDP"))

	// Advanced checkbox — placed above sliders so toggle swaps content in-place.
	w.tdpAdvancedCheck = gtk.NewCheckButtonWithLabel("Advanced")
	w.tdpAdvancedCheck.AddCSSClass("advanced-check")
	if w.gamescope {
		addTouchActivate(w.tdpAdvancedCheck, func() { w.tdpAdvancedCheck.SetActive(!w.tdpAdvancedCheck.Active()) })
	}
	content.Append(w.tdpAdvancedCheck)

	// Basic TDP box (visible by default).
	tdpBasicBox := gtk.NewBox(gtk.OrientationVertical, 4)
	w.tdpBasicScale = gtk.NewScaleWithRange(gtk.OrientationHorizontal, float64(w.limits.TDPMin), float64(w.limits.BasicSliderMax()), 1)
	w.tdpBasicScale.SetDigits(0)
	w.tdpBasicScale.SetDrawValue(false)
	w.tdpBasicScale.SetValue(float64(50))
	w.tdpBasicScale.SetFocusable(false)
	w.tdpBasicLabel = gtk.NewLabel("50 W")
	w.tdpBasicLabel.AddCSSClass("scale-value")
	w.tdpBasicScale.ConnectValueChanged(func() {
		w.tdpBasicLabel.SetLabel(fmt.Sprintf("%d W", int(w.tdpBasicScale.Value())))
	})
	tdpBasicBox.Append(w.tdpBasicScale)
	tdpBasicBox.Append(w.tdpBasicLabel)
	content.Append(tdpBasicBox)

	// Advanced box (hidden by default) — replaces basic slider in-place.
	w.tdpAdvancedBox = gtk.NewBox(gtk.OrientationVertical, 4)
	w.tdpAdvancedBox.SetVisible(false)

	w.tdpWarningLabel = gtk.NewLabel(fmt.Sprintf(
		"WARNING: Values above %dW may cause thermal throttling, instability, or hardware damage. Use at your own risk — we are not responsible for any damages.",
		w.limits.TDPMaxSafe))
	w.tdpWarningLabel.SetWrap(true)
	w.tdpWarningLabel.SetHAlign(gtk.AlignStart)
	w.tdpWarningLabel.AddCSSClass("tdp-warning")
	w.tdpAdvancedBox.Append(w.tdpWarningLabel)

	w.tdpPL1Scale, w.tdpPL1Label = w.buildTdpScale("PL1 (SPL)", "Sustained power limit — the long-term average power the CPU targets.")
	w.tdpPL2Scale, w.tdpPL2Label = w.buildTdpScale("PL2 (SPPT)", "Short boost — maximum power during brief burst workloads.")
	w.tdpPL3Scale, w.tdpPL3Label = w.buildTdpScale("PL3 (FPPT)", "Fast boost — peak instantaneous power for single-threaded spikes.")

	// --- UNDERVOLT (inside advanced box) ---
	w.uvBox = gtk.NewBox(gtk.OrientationVertical, 4)
	// Hidden by default; syncCustomView shows it when UndervoltAvailable.
	w.uvBox.SetVisible(false)

	w.uvBox.Append(sectionLabel("UNDERVOLT"))

	uvWarn := gtk.NewLabel("Undervolt offsets are only active while the Custom profile is selected. Switching to a stock profile resets them to 0. Unstable values may cause crashes.")
	uvWarn.SetWrap(true)
	uvWarn.SetHAlign(gtk.AlignStart)
	uvWarn.AddCSSClass("tdp-warning")
	w.uvBox.Append(uvWarn)

	w.uvCpuScale, w.uvCpuLabel = w.buildUvScale("CPU Curve Optimizer", -40, 0)

	// UV buttons: Save UV | Reset UV
	uvBtnRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	uvBtnRow.AddCSSClass("custom-actions")

	w.saveUvBtn = gtk.NewButtonWithLabel("Save UV")
	w.saveUvBtn.AddCSSClass("save-btn")
	w.saveUvBtn.SetHExpand(true)
	w.saveUvBtn.ConnectClicked(func() { w.saveUndervolt() })
	uvBtnRow.Append(w.saveUvBtn)

	w.resetUvBtn = gtk.NewButtonWithLabel("Reset UV")
	w.resetUvBtn.SetHExpand(true)
	w.resetUvBtn.ConnectClicked(func() { w.resetUndervolt() })
	uvBtnRow.Append(w.resetUvBtn)

	w.uvBox.Append(uvBtnRow)
	w.tdpAdvancedBox.Append(w.uvBox)

	content.Append(w.tdpAdvancedBox)

	w.tdpAdvancedCheck.ConnectToggled(func() {
		adv := w.tdpAdvancedCheck.Active()
		w.tdpAdvancedBox.SetVisible(adv)
		tdpBasicBox.SetVisible(!adv)
	})

	content.Append(separator())

	// --- FAN CURVE ---
	content.Append(sectionLabel("FAN CURVE"))
	w.fanCurve = w.newFanCurveEditor()
	content.Append(w.fanCurve.area)

	content.Append(separator())

	// --- BUTTONS ---
	// Save row: Save TDP | Save Fans | Save Both
	saveRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	saveRow.AddCSSClass("custom-actions")

	w.saveTdpBtn = gtk.NewButtonWithLabel("Save TDP")
	w.saveTdpBtn.AddCSSClass("save-btn")
	w.saveTdpBtn.SetHExpand(true)
	w.saveTdpBtn.ConnectClicked(func() { w.saveCustomTdp() })
	saveRow.Append(w.saveTdpBtn)

	w.saveFanBtn = gtk.NewButtonWithLabel("Save Fans")
	w.saveFanBtn.AddCSSClass("save-btn")
	w.saveFanBtn.SetHExpand(true)
	w.saveFanBtn.ConnectClicked(func() { w.saveCustomFanCurve() })
	saveRow.Append(w.saveFanBtn)

	w.saveBothBtn = gtk.NewButtonWithLabel("Save Both")
	w.saveBothBtn.AddCSSClass("save-btn")
	w.saveBothBtn.SetHExpand(true)
	w.saveBothBtn.ConnectClicked(func() { w.saveCustomBoth() })
	saveRow.Append(w.saveBothBtn)

	content.Append(saveRow)

	// Reset row: Reset TDP | Reset Fans
	resetRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	resetRow.AddCSSClass("custom-actions")

	w.resetTdpBtn = gtk.NewButtonWithLabel("Reset TDP")
	w.resetTdpBtn.SetHExpand(true)
	w.resetTdpBtn.ConnectClicked(func() { w.resetTdp() })
	resetRow.Append(w.resetTdpBtn)

	w.resetFanBtn = gtk.NewButtonWithLabel("Reset Fans")
	w.resetFanBtn.SetHExpand(true)
	w.resetFanBtn.ConnectClicked(func() { w.resetFanCurve() })
	resetRow.Append(w.resetFanBtn)

	content.Append(resetRow)

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetVExpand(true)
	scroll.SetChild(content)
	w.customScroll = scroll
	view.Append(scroll)

	return view
}

// buildTdpScale creates a labeled TDP slider (5–93W) and appends it to the
// advanced box. Returns the scale and value label.
func (w *Window) buildTdpScale(label, desc string) (*gtk.Scale, *gtk.Label) {
	nameLabel := gtk.NewLabel(label)
	nameLabel.SetHAlign(gtk.AlignStart)
	nameLabel.AddCSSClass("scale-name")
	w.tdpAdvancedBox.Append(nameLabel)
	descLabel := gtk.NewLabel(desc)
	descLabel.SetHAlign(gtk.AlignStart)
	descLabel.SetWrap(true)
	descLabel.AddCSSClass("scale-value")
	w.tdpAdvancedBox.Append(descLabel)
	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, float64(w.limits.TDPMin), float64(w.limits.TDPMaxForced), 1)
	sc.SetDigits(0)
	sc.SetDrawValue(false)
	sc.SetValue(50)
	sc.SetFocusable(false)
	valLabel := gtk.NewLabel("50 W")
	valLabel.AddCSSClass("scale-value")
	sc.ConnectValueChanged(func() {
		valLabel.SetLabel(fmt.Sprintf("%d W", int(sc.Value())))
	})
	w.tdpAdvancedBox.Append(sc)
	w.tdpAdvancedBox.Append(valLabel)
	return sc, valLabel
}

// buildUvScale creates a labeled undervolt slider and appends it to uvBox.
func (w *Window) buildUvScale(label string, lo, hi float64) (*gtk.Scale, *gtk.Label) {
	nameLabel := gtk.NewLabel(label)
	nameLabel.SetHAlign(gtk.AlignStart)
	nameLabel.AddCSSClass("scale-name")
	w.uvBox.Append(nameLabel)
	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, lo, hi, 1)
	sc.SetDigits(0)
	sc.SetDrawValue(false)
	sc.SetValue(0)
	sc.SetFocusable(false)
	valLabel := gtk.NewLabel(uvLabel(label, 0))
	valLabel.AddCSSClass("scale-value")
	sc.ConnectValueChanged(func() {
		valLabel.SetLabel(uvLabel(label, int(sc.Value())))
	})
	w.uvBox.Append(sc)
	w.uvBox.Append(valLabel)
	return sc, valLabel
}

// uvLabel formats an undervolt value label, e.g. "CPU Curve Optimizer: -20" or "... 0 (stock)".
func uvLabel(name string, val int) string {
	if val == 0 {
		return fmt.Sprintf("%s: 0 (stock)", name)
	}
	return fmt.Sprintf("%s: %d", name, val)
}

// syncCustomView populates the custom view widgets from daemon state.
func (w *Window) syncCustomView() {
	if w.state == nil {
		return
	}
	prev := w.syncing
	w.syncing = true
	defer func() { w.syncing = prev }()

	// TDP.
	if w.state.TDP != nil {
		tdp := w.state.TDP
		if w.tdpBasicScale != nil {
			v := float64(tdp.PL1SPL)
			if m := float64(w.limits.BasicSliderMax()); v > m {
				v = m
			}
			w.tdpBasicScale.SetValue(v)
			w.tdpBasicLabel.SetLabel(fmt.Sprintf("%d W", int(v)))
		}
		if w.tdpPL1Scale != nil {
			w.tdpPL1Scale.SetValue(float64(tdp.PL1SPL))
			w.tdpPL1Label.SetLabel(fmt.Sprintf("%d W", tdp.PL1SPL))
		}
		if w.tdpPL2Scale != nil {
			w.tdpPL2Scale.SetValue(float64(tdp.PL2SPPT))
			w.tdpPL2Label.SetLabel(fmt.Sprintf("%d W", tdp.PL2SPPT))
		}
		if w.tdpPL3Scale != nil {
			w.tdpPL3Scale.SetValue(float64(tdp.FPPT))
			w.tdpPL3Label.SetLabel(fmt.Sprintf("%d W", tdp.FPPT))
		}

		// Switch to the advanced view when the applied TDP cannot be expressed in
		// basic mode. Otherwise the basic slider silently clamps and its label
		// reports the clamped number, so the drawer claims 70W while the hardware
		// runs at 80W. Only ever forced on, never off: once the user unchecks it
		// that is a deliberate choice to edit in basic terms.
		if w.tdpAdvancedCheck != nil && !w.tdpAdvancedCheck.Active() &&
			w.limits.NeedsAdvanced(w.state.Profile, *tdp) {
			w.tdpAdvancedCheck.SetActive(true)
		}
	}

	// Fan curve. Only adopt the daemon's points when the fans are actually
	// following them: on a stock profile the fans are released to firmware auto
	// but the curve registers still read back the last custom curve, so copying
	// them unconditionally left the editor showing a curve that was not in force.
	// Redraw either way — the PWM floor line depends on PL1, which may have just
	// changed.
	if w.fanCurve != nil {
		if power.FanCurveIsCustom(w.state.FanCurve) {
			copy(w.fanCurve.points[:], w.state.FanCurve.Points)
		} else {
			w.fanCurve.points = w.limits.DefaultCurve()
		}
	}
	if w.fanCurve != nil {
		// A curve saved while the floor was off can sit below it once a high TDP
		// is applied. Lift it so what is drawn is what the daemon would accept.
		w.fanCurve.enforceConstraints(0)
		w.fanCurve.area.QueueDraw()
	}

	// Reset Fans is refused by the daemon while the high-TDP floor is in force —
	// firmware auto has no floor, so releasing the fans there would remove the
	// protection the power limit requires. Reset TDP is the way out.
	if w.resetFanBtn != nil {
		floored := w.fanFloorPWM() > 0
		w.resetFanBtn.SetSensitive(!floored)
		if floored {
			w.resetFanBtn.SetTooltipText(fmt.Sprintf(
				"Unavailable while sustained TDP is above %dW — fans must stay at %d%% minimum. Use Reset TDP first.",
				w.limits.TDPMaxSafe, w.limits.HighTDPMinPWM*100/power.PWMMax))
		} else {
			w.resetFanBtn.SetTooltipText("Reset fan curves to firmware auto")
		}
	}

	// Undervolt.
	if w.uvBox != nil {
		w.uvBox.SetVisible(w.state.UndervoltAvailable)
	}
	cpuCO := 0
	if w.state.Undervolt != nil && w.state.Profile == "custom" {
		cpuCO = w.state.Undervolt.CPUCO
	}
	if w.uvCpuScale != nil {
		w.uvCpuScale.SetValue(float64(cpuCO))
		w.uvCpuLabel.SetLabel(uvLabel("CPU Curve Optimizer", cpuCO))
	}

	// Telemetry.
	if w.telemetryTempLabel != nil {
		w.telemetryTempLabel.SetLabel(fmt.Sprintf("APU: %d°C", w.state.Temperature))
	}
	if w.telemetryFanLabel != nil {
		w.telemetryFanLabel.SetLabel(fmt.Sprintf("Fan: %d RPM", w.state.FanRPM))
	}
}

// sendTdp sends the current TDP slider values to the daemon.
func (w *Window) sendTdp() error {
	if w.tdpAdvancedCheck != nil && w.tdpAdvancedCheck.Active() {
		pl1 := fmt.Sprintf("%d", int(w.tdpPL1Scale.Value()))
		pl2 := fmt.Sprintf("%d", int(w.tdpPL2Scale.Value()))
		pl3 := fmt.Sprintf("%d", int(w.tdpPL3Scale.Value()))
		maxPL := int(math.Max(w.tdpPL1Scale.Value(), math.Max(w.tdpPL2Scale.Value(), w.tdpPL3Scale.Value())))
		force := w.limits.ForceRequired(maxPL)
		_, err := api.SendTdpSet(pl1, pl1, pl2, pl3, force)
		return err
	}
	watts := fmt.Sprintf("%d", int(w.tdpBasicScale.Value()))
	_, err := api.SendTdpSet(watts, "", "", "", false)
	return err
}

// sendFanCurve sends the current fan curve to the daemon.
func (w *Window) sendFanCurve() error {
	if w.fanCurve == nil {
		return nil
	}
	_, err := api.SendFanCurveSet(w.fanCurve.curveString())
	return err
}

// refreshProfile fetches state and updates the profile button highlight.
// refreshState fetches daemon state and re-syncs both the custom view and the
// profile buttons. Every custom-profile operation uses it rather than syncing the
// profile alone: the fan curve editor's PWM floor is derived from the applied
// PL1, so a TDP change has to re-evaluate the whole view, not just the highlight.
// Safe to call from a background goroutine.
func (w *Window) refreshState() {
	ok, state, err := api.SendGetState()
	if !ok || err != nil {
		return
	}
	glib.IdleAdd(func() {
		w.state = state
		w.syncCustomView()
		w.syncing = true
		w.syncProfile()
		w.syncing = false
	})
}

// saveCustomTdp commits only the TDP values.
func (w *Window) saveCustomTdp() {
	go func() {
		if err := w.sendTdp(); err != nil {
			w.reportError("Save TDP", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("custom TDP saved")
		w.refreshState()
	}()
}

// saveCustomFanCurve commits only the fan curve.
func (w *Window) saveCustomFanCurve() {
	go func() {
		if err := w.sendFanCurve(); err != nil {
			w.reportError("Save fan curve", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("custom fan curve saved")
		w.refreshState()
	}()
}

// saveCustomBoth commits both TDP and fan curve.
func (w *Window) saveCustomBoth() {
	go func() {
		tdpErr := w.sendTdp()
		fanErr := w.sendFanCurve()
		switch {
		case tdpErr != nil:
			// TDP first: a rejected TDP is usually why the fan write failed too
			// (the daemon refuses a curve below the 80% floor while PL1 is high).
			w.reportError("Save TDP", tdpErr)
		case fanErr != nil:
			w.reportError("Save fan curve", fanErr)
		default:
			w.clearErrorAsync()
			slog.Info("custom profile saved (TDP + fans)")
		}
		w.refreshState()
	}()
}

// resetTdp resets TDP to firmware defaults.
func (w *Window) resetTdp() {
	go func() {
		if _, err := api.SendTdpReset(); err != nil {
			w.reportError("Reset TDP", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("tdp reset to defaults")
		w.refreshState()
	}()
}

// resetFanCurve resets fan curves to firmware auto mode.
func (w *Window) resetFanCurve() {
	go func() {
		if _, err := api.SendFanCurveReset(); err != nil {
			// The daemon refuses this while sustained TDP is above the safe max —
			// firmware auto has no PWM floor. Reset TDP is the way out.
			w.reportError("Reset fans", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("fan curve reset to auto")
		w.refreshState()
	}()
}

// saveUndervolt commits the current Curve Optimizer offsets to the daemon.
func (w *Window) saveUndervolt() {
	go func() {
		cpu := fmt.Sprintf("%d", int(w.uvCpuScale.Value()))
		if _, err := api.SendUndervoltSet(cpu); err != nil {
			w.reportError("Save undervolt", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("undervolt saved", "cpu", cpu)
		w.refreshState()
	}()
}

// resetUndervolt resets Curve Optimizer to stock (0).
func (w *Window) resetUndervolt() {
	go func() {
		if _, err := api.SendUndervoltReset(); err != nil {
			w.reportError("Reset undervolt", err)
			return
		}
		w.clearErrorAsync()
		slog.Info("undervolt reset to stock")
		w.refreshState()
	}()
}

// startTelemetryPolling begins polling the daemon for APU temp and fan RPM
// every second while the drawer is visible. Updates the header telemetry
// label on all views, and also updates custom view labels + fan curve
// indicator when the custom view is active.
func (w *Window) startTelemetryPolling() {
	w.telemetryGen++
	gen := w.telemetryGen
	glib.TimeoutAdd(1000, func() bool {
		if gen != w.telemetryGen || !w.visible {
			return false
		}
		go func() {
			ok, state, err := api.SendGetState()
			if !ok || err != nil {
				return
			}
			glib.IdleAdd(func() {
				if gen != w.telemetryGen {
					return
				}
				w.state = state

				// Header telemetry (visible on all views).
				if w.headerTelemetry != nil {
					w.headerTelemetry.SetLabel(fmt.Sprintf("%d°C · %d RPM", state.Temperature, state.FanRPM))
				}

				// Custom view telemetry (only when active).
				if w.viewStack != nil && w.viewStack.VisibleChildName() == "custom" {
					if w.telemetryTempLabel != nil {
						w.telemetryTempLabel.SetLabel(fmt.Sprintf("APU: %d°C", state.Temperature))
					}
					if w.telemetryFanLabel != nil {
						w.telemetryFanLabel.SetLabel(fmt.Sprintf("Fan: %d RPM", state.FanRPM))
					}
					if w.fanCurve != nil {
						w.fanCurve.area.QueueDraw()
					}
				}
			})
		}()
		return true
	})
}

// buildCustomFocusList builds the 2D focus grid for the custom profile view.
func (w *Window) buildCustomFocusList() {
	var items []focusItem

	// Row 0: back button.
	items = append(items, focusItem{
		widget: w.customBackBtn, row: 0, col: 0,
		section:    "nav",
		onActivate: func() { w.showMainView() },
	})

	// Row 1: basic TDP slider.
	if w.tdpBasicScale != nil {
		oL, oR, gV, sV := scaleAdjust(w.tdpBasicScale, 5)
		items = append(items, focusItem{
			widget: w.tdpBasicScale, row: 1, col: 0,
			section:  "tdp",
			editable: true,
			onLeft:   oL, onRight: oR,
			getValue: gV, setValue: sV,
			isVisible: func() bool { return w.tdpBasicScale.IsVisible() },
		})
	}

	// Row 2: advanced checkbox.
	if w.tdpAdvancedCheck != nil {
		items = append(items, focusItem{
			widget: w.tdpAdvancedCheck, row: 2, col: 0,
			section:    "tdp",
			onActivate: func() { w.tdpAdvancedCheck.SetActive(!w.tdpAdvancedCheck.Active()) },
		})
	}

	// Rows 3-5: PL1/PL2/PL3 sliders.
	advVis := func() bool { return w.tdpAdvancedBox.IsVisible() }
	for i, sc := range []*gtk.Scale{w.tdpPL1Scale, w.tdpPL2Scale, w.tdpPL3Scale} {
		oL, oR, gV, sV := scaleAdjust(sc, 1)
		items = append(items, focusItem{
			widget: sc, row: 3 + i, col: 0,
			section:   "tdp",
			editable:  true,
			isVisible: advVis,
			onLeft:    oL, onRight: oR,
			getValue: gV, setValue: sV,
		})
	}

	// Row 6: fan curve (editable with custom behavior).
	if w.fanCurve != nil {
		items = append(items, focusItem{
			widget: w.fanCurve.area, row: 6, col: 0,
			section: "fan",
			// Fan curve is navigable but not editable via gamepad in this first pass.
			// Touch/mouse drag handles interaction.
		})
	}

	// Rows 7-9: undervolt (visible only when available).
	uvVis := func() bool { return w.tdpAdvancedBox.IsVisible() && w.uvBox != nil && w.uvBox.IsVisible() }
	if w.uvCpuScale != nil {
		oL, oR, gV, sV := scaleAdjust(w.uvCpuScale, 1)
		items = append(items, focusItem{
			widget: w.uvCpuScale, row: 7, col: 0,
			section: "undervolt", editable: true, isVisible: uvVis,
			onLeft: oL, onRight: oR, getValue: gV, setValue: sV,
		})
	}
	items = append(items, focusItem{
		widget: w.saveUvBtn, row: 8, col: 0,
		section: "undervolt", isVisible: uvVis,
		onActivate: func() { w.saveUvBtn.Activate() },
	})
	items = append(items, focusItem{
		widget: w.resetUvBtn, row: 8, col: 1,
		section: "undervolt", isVisible: uvVis,
		onActivate: func() { w.resetUvBtn.Activate() },
	})

	// Row 9: save buttons.
	items = append(items, focusItem{
		widget: w.saveTdpBtn, row: 9, col: 0,
		section:    "actions",
		onActivate: func() { w.saveTdpBtn.Activate() },
	})
	items = append(items, focusItem{
		widget: w.saveFanBtn, row: 9, col: 1,
		section:    "actions",
		onActivate: func() { w.saveFanBtn.Activate() },
	})
	items = append(items, focusItem{
		widget: w.saveBothBtn, row: 9, col: 2,
		section:    "actions",
		onActivate: func() { w.saveBothBtn.Activate() },
	})

	// Row 10: reset buttons.
	items = append(items, focusItem{
		widget: w.resetTdpBtn, row: 10, col: 0,
		section:    "actions",
		onActivate: func() { w.resetTdpBtn.Activate() },
	})
	items = append(items, focusItem{
		widget: w.resetFanBtn, row: 10, col: 1,
		section:    "actions",
		onActivate: func() { w.resetFanBtn.Activate() },
	})

	w.customFocusItems = items
}
