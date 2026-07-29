// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

package power

import (
	"testing"

	"github.com/dahui/z13ctl/api"
)

// otherDevice is a fictional second device with a deliberately different
// envelope: a much lower power ceiling, a narrower fan temperature range, no fan
// floor at all, and its own stock profile table.
//
// Every rule is exercised against both it and the Z13 so that nothing quietly
// depends on the Z13's numbers. That is the regression the multi-device port
// would otherwise walk into.
func otherDevice() Limits {
	return Limits{
		Model:         "FICTIONAL-1",
		TDPMin:        4,
		TDPMaxSafe:    30,
		TDPMaxForced:  45,
		HighTDPMinPWM: 0, // this device has no high-TDP fan floor
		TempMin:       40,
		TempMax:       90,
		StockProfilePPT: map[string]api.TDPState{
			"quiet":       {PL1SPL: 10, PL2SPPT: 15, FPPT: 15},
			"balanced":    {PL1SPL: 20, PL2SPPT: 28, FPPT: 26},
			"performance": {PL1SPL: 30, PL2SPPT: 40, FPPT: 40},
		},
	}
}

func TestDefaultLimitsAreSelfConsistent(t *testing.T) {
	for _, l := range []Limits{DefaultLimits(), otherDevice()} {
		if l.TDPMin >= l.TDPMaxSafe {
			t.Errorf("%s: TDPMin %d not below TDPMaxSafe %d", l.Model, l.TDPMin, l.TDPMaxSafe)
		}
		if l.TDPMaxSafe > l.TDPMaxForced {
			t.Errorf("%s: TDPMaxSafe %d above TDPMaxForced %d", l.Model, l.TDPMaxSafe, l.TDPMaxForced)
		}
		if l.TempMin >= l.TempMax {
			t.Errorf("%s: TempMin %d not below TempMax %d", l.Model, l.TempMin, l.TempMax)
		}
		// A curve needs one degree per point.
		if got := l.TempMax - l.TempMin; got < CurvePoints {
			t.Errorf("%s: temperature range %d too narrow for %d points", l.Model, got, CurvePoints)
		}
		if b := l.BasicSliderMax(); b <= l.TDPMin || b > l.TDPMaxSafe {
			t.Errorf("%s: BasicSliderMax %d outside (%d, %d]", l.Model, b, l.TDPMin, l.TDPMaxSafe)
		}
	}
}

func TestBasicSliderMaxIsDerivedNotFixed(t *testing.T) {
	// The Z13's historic hardcoded value, preserved by the derivation.
	if got := DefaultLimits().BasicSliderMax(); got != 70 {
		t.Errorf("Z13 BasicSliderMax = %d, want 70", got)
	}
	if got := otherDevice().BasicSliderMax(); got != 25 {
		t.Errorf("fictional BasicSliderMax = %d, want 25", got)
	}
	// A device whose safe max leaves no headroom must not produce an inverted
	// or below-minimum range.
	tiny := Limits{TDPMin: 5, TDPMaxSafe: 6, TDPMaxForced: 10}
	if got := tiny.BasicSliderMax(); got < tiny.TDPMin {
		t.Errorf("tiny BasicSliderMax = %d, below TDPMin %d", got, tiny.TDPMin)
	}
}

func TestSanitizedFillsUnsetFields(t *testing.T) {
	d := DefaultLimits()
	got := Limits{}.Sanitized()

	if got.TDPMin != d.TDPMin || got.TDPMaxSafe != d.TDPMaxSafe || got.TDPMaxForced != d.TDPMaxForced {
		t.Errorf("zero Limits did not inherit TDP defaults: %+v", got)
	}
	if got.TempMin != d.TempMin || got.TempMax != d.TempMax {
		t.Errorf("zero Limits did not inherit temperature defaults: %+v", got)
	}
	if len(got.StockProfilePPT) != len(d.StockProfilePPT) {
		t.Errorf("zero Limits did not inherit the stock profile table")
	}
	if got.Model == "" {
		t.Error("zero Limits did not inherit a model name")
	}
}

func TestSanitizedKeepsProvidedValues(t *testing.T) {
	o := otherDevice()
	got := o.Sanitized()

	if got.TDPMaxSafe != o.TDPMaxSafe || got.TempMax != o.TempMax || got.Model != o.Model {
		t.Errorf("Sanitized overwrote provided values: %+v", got)
	}
	// Zero is a legitimate HighTDPMinPWM — a device with no fan floor. It must
	// not be "helpfully" replaced with the Z13's 204, which would clamp fan
	// curves on hardware that has no such rule.
	if got.HighTDPMinPWM != 0 {
		t.Errorf("HighTDPMinPWM = %d, want 0 preserved (no floor on this device)", got.HighTDPMinPWM)
	}
}

func TestForceRequired(t *testing.T) {
	for _, l := range []Limits{DefaultLimits(), otherDevice()} {
		if l.ForceRequired(l.TDPMaxSafe) {
			t.Errorf("%s: ForceRequired(%d) = true, want false at the threshold", l.Model, l.TDPMaxSafe)
		}
		if !l.ForceRequired(l.TDPMaxSafe + 1) {
			t.Errorf("%s: ForceRequired(%d) = false, want true above it", l.Model, l.TDPMaxSafe+1)
		}
	}
}

func TestFanFloorPWM(t *testing.T) {
	z := DefaultLimits()
	for _, tt := range []struct{ pl1, want int }{
		{pl1: 0, want: PWMMin},
		{pl1: 50, want: PWMMin},
		{pl1: z.TDPMaxSafe, want: PWMMin},              // at the threshold, unconstrained
		{pl1: z.TDPMaxSafe + 1, want: z.HighTDPMinPWM}, // one over, floor applies
		{pl1: z.TDPMaxForced, want: z.HighTDPMinPWM},
	} {
		if got := z.FanFloorPWM(tt.pl1); got != tt.want {
			t.Errorf("Z13 FanFloorPWM(%d) = %d, want %d", tt.pl1, got, tt.want)
		}
	}

	// A device declaring no floor must never clamp, even far above its safe max.
	o := otherDevice()
	if got := o.FanFloorPWM(o.TDPMaxForced); got != PWMMin {
		t.Errorf("fictional FanFloorPWM(%d) = %d, want %d (device has no floor)", o.TDPMaxForced, got, PWMMin)
	}
}

func TestIsStockPPT(t *testing.T) {
	for _, l := range []Limits{DefaultLimits(), otherDevice()} {
		for name, stock := range l.StockProfilePPT {
			if !l.IsStockPPT(stock) {
				t.Errorf("%s: IsStockPPT(%s defaults %+v) = false, want true", l.Model, name, stock)
			}
			// The daemon mirrors APU/Platform sPPT from PL2, so a real reading
			// carries values the table does not list. They must not affect it.
			withMirrored := stock
			withMirrored.APUSPPT = stock.PL2SPPT
			withMirrored.PlatformSPPT = stock.PL2SPPT
			if !l.IsStockPPT(withMirrored) {
				t.Errorf("%s: IsStockPPT(%s with mirrored APU fields) = false, want true", l.Model, name)
			}
		}
	}

	z := DefaultLimits()
	for _, tdp := range []api.TDPState{
		{PL1SPL: 80, PL2SPPT: 80, FPPT: 80},
		{PL1SPL: 52, PL2SPPT: 71, FPPT: 69}, // one watt off balanced
		{PL1SPL: 45, PL2SPPT: 45, FPPT: 45},
		{},
	} {
		if z.IsStockPPT(tdp) {
			t.Errorf("IsStockPPT(%+v) = true, want false", tdp)
		}
	}

	// Another device's stock values are not this device's stock values.
	if z.IsStockPPT(otherDevice().StockProfilePPT["balanced"]) {
		t.Error("Z13 accepted the fictional device's balanced defaults as stock")
	}
}

func TestNeedsAdvanced(t *testing.T) {
	z := DefaultLimits()

	tests := []struct {
		name    string
		profile string
		tdp     api.TDPState
		want    bool
	}{
		{
			// The regression this function exists for: PL1 above the basic
			// slider's ceiling used to clamp to 70 and report "70 W" while the
			// hardware ran at 80 W, and a save would then send 70.
			name:    "sustained above basic ceiling",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 80, PL2SPPT: 80, FPPT: 80},
			want:    true,
		},
		{
			name:    "exactly at the basic ceiling is representable",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 70, PL2SPPT: 70, FPPT: 70},
			want:    false,
		},
		{
			name:    "one over the basic ceiling is not",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 71, PL2SPPT: 71, FPPT: 71},
			want:    true,
		},
		{
			// Basic mode applies one value to all three, so differing limits
			// cannot be shown even when every value is inside the basic range.
			// Deliberately not 52/71/70 — that is the balanced stock triple and
			// is covered below as a state the user did not choose.
			name:    "limits differ below the ceiling",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 50, PL2SPPT: 65, FPPT: 60},
			want:    true,
		},
		{
			name:    "only PL2 differs",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 50, PL2SPPT: 60, FPPT: 50},
			want:    true,
		},
		{
			name:    "only PL3 differs",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 50, PL2SPPT: 50, FPPT: 60},
			want:    true,
		},
		{
			// What a basic save round-trips as: the daemon defaults the blank
			// PL fields to the single value, so this must stay in basic.
			name:    "equal triple from a basic save",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 45, PL2SPPT: 45, FPPT: 45},
			want:    false,
		},
		{
			// APUSPPT/PlatformSPPT are daemon bookkeeping mirrored from PL2 and
			// are not shown in the drawer, so they must not force advanced.
			name:    "mirrored APU fields are ignored",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 50, PL2SPPT: 50, FPPT: 50, APUSPPT: 70, PlatformSPPT: 70},
			want:    false,
		},
		{
			name:    "zero value is representable",
			profile: ProfileCustom,
			tdp:     api.TDPState{},
			want:    false,
		},

		// Stock profiles report the firmware's own per-profile PPT defaults,
		// which differ between limits by design. Those are a starting point for
		// editing, not settings the user chose, so they must never force the
		// advanced view — otherwise every stock profile opens it.
		{
			name:    "stock balanced defaults do not force advanced",
			profile: "balanced",
			tdp:     api.TDPState{PL1SPL: 52, PL2SPPT: 71, FPPT: 70},
			want:    false,
		},
		{
			name:    "stock quiet defaults do not force advanced",
			profile: "quiet",
			tdp:     api.TDPState{PL1SPL: 40, PL2SPPT: 55, FPPT: 55},
			want:    false,
		},
		{
			name:    "stock performance defaults do not force advanced",
			profile: "performance",
			tdp:     api.TDPState{PL1SPL: 70, PL2SPPT: 86, FPPT: 86},
			want:    false,
		},
		{
			// Even a reading above the ceiling stays basic on a stock profile:
			// it is the firmware's value, not a saved custom one.
			name:    "stock profile above the ceiling still stays basic",
			profile: "performance",
			tdp:     api.TDPState{PL1SPL: 80, PL2SPPT: 90, FPPT: 90},
			want:    false,
		},
		{
			name:    "empty profile is not custom",
			profile: "",
			tdp:     api.TDPState{PL1SPL: 80, PL2SPPT: 80, FPPT: 80},
			want:    false,
		},

		// Saving a fan curve or undervolt flips the daemon's profile to custom
		// by itself, while the power limits stay at the firmware's values. The
		// profile check alone would then fire on numbers the user never chose.
		{
			name:    "custom fan curve on top of stock balanced limits",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 52, PL2SPPT: 71, FPPT: 70},
			want:    false,
		},
		{
			name:    "custom fan curve on top of stock quiet limits",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 40, PL2SPPT: 55, FPPT: 55},
			want:    false,
		},
		{
			name:    "custom fan curve on top of stock performance limits",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 70, PL2SPPT: 86, FPPT: 86},
			want:    false,
		},
		{
			// One watt off the stock table is a deliberate edit, not firmware.
			name:    "one watt off stock balanced is a user choice",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 53, PL2SPPT: 71, FPPT: 70},
			want:    true,
		},
		{
			// The case the ceiling rule exists for must survive the stock check.
			name:    "advanced burst limits within the basic ceiling",
			profile: ProfileCustom,
			tdp:     api.TDPState{PL1SPL: 65, PL2SPPT: 85, FPPT: 90},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := z.NeedsAdvanced(tt.profile, tt.tdp); got != tt.want {
				t.Errorf("NeedsAdvanced(%q, %+v) = %v, want %v", tt.profile, tt.tdp, got, tt.want)
			}
		})
	}
}

// The ceiling that matters is the device's own, not the Z13's.
func TestNeedsAdvancedUsesTheDevicesCeiling(t *testing.T) {
	o := otherDevice() // BasicSliderMax 25

	// Well inside the Z13's basic range, but above this device's.
	tdp := api.TDPState{PL1SPL: 40, PL2SPPT: 40, FPPT: 40}
	if !o.NeedsAdvanced(ProfileCustom, tdp) {
		t.Errorf("fictional device: NeedsAdvanced(%+v) = false, want true (over its %dW ceiling)",
			tdp, o.BasicSliderMax())
	}
	if DefaultLimits().NeedsAdvanced(ProfileCustom, tdp) {
		t.Errorf("Z13: NeedsAdvanced(%+v) = true, want false (inside its %dW ceiling)",
			tdp, DefaultLimits().BasicSliderMax())
	}

	// And this device's own stock values must not force advanced on it.
	if o.NeedsAdvanced(ProfileCustom, o.StockProfilePPT["balanced"]) {
		t.Error("fictional device forced advanced for its own stock balanced values")
	}
}

func TestFanCurveIsCustom(t *testing.T) {
	eight := make([]api.FanCurvePoint, CurvePoints)

	tests := []struct {
		name string
		fc   *api.FanCurveState
		want bool
	}{
		{name: "nil state", fc: nil, want: false},
		{
			name: "custom mode with a full curve",
			fc:   &api.FanCurveState{Mode: FanModeCustom, Points: eight},
			want: true,
		},
		{
			// The regression: switching to a stock profile releases the fans to
			// firmware auto, but the curve registers still read back the old
			// custom points. Displaying them shows a curve the fans do not follow.
			name: "auto mode still reports stale points",
			fc:   &api.FanCurveState{Mode: FanModeAuto, Points: eight},
			want: false,
		},
		{
			name: "full-speed mode",
			fc:   &api.FanCurveState{Mode: FanModeFullSpeed, Points: eight},
			want: false,
		},
		{
			name: "custom mode but a short curve is unusable",
			fc:   &api.FanCurveState{Mode: FanModeCustom, Points: eight[:3]},
			want: false,
		},
		{
			name: "custom mode with no points",
			fc:   &api.FanCurveState{Mode: FanModeCustom},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FanCurveIsCustom(tt.fc); got != tt.want {
				t.Errorf("FanCurveIsCustom() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultCurveIsValid(t *testing.T) {
	for _, l := range []Limits{DefaultLimits(), otherDevice()} {
		assertCurveValid(t, l, l.DefaultCurve(), PWMMin)
	}
}

// The Z13's hand-tuned shape must survive being fitted to the Z13's own range.
func TestDefaultCurveUnchangedOnTheZ13(t *testing.T) {
	want := "35:0,45:25,50:50,60:80,70:120,80:170,90:220,100:255"
	if got := DefaultLimits().DefaultCurve().String(); got != want {
		t.Errorf("Z13 DefaultCurve() = %q, want %q", got, want)
	}
}

// A curve built for a wider temperature range must come back valid rather than
// collapsing every out-of-range point onto the maximum.
func TestEnforceCurve_NormalizesACurveFromAnotherDevice(t *testing.T) {
	narrow := otherDevice()
	c := DefaultLimits().DefaultCurve() // 35–100°C, outside narrow's 40–90
	narrow.EnforceCurve(&c, 0, PWMMin)
	assertCurveValid(t, narrow, c, PWMMin)
}

func TestCurveString(t *testing.T) {
	c := Curve{
		{Temp: 35, PWM: 0}, {Temp: 45, PWM: 25}, {Temp: 50, PWM: 50}, {Temp: 60, PWM: 80},
		{Temp: 70, PWM: 120}, {Temp: 80, PWM: 170}, {Temp: 90, PWM: 220}, {Temp: 100, PWM: 255},
	}
	want := "35:0,45:25,50:50,60:80,70:120,80:170,90:220,100:255"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// Dragging a point off the left edge must not squash the points behind it onto
// TempMin. Point 3 has three points below it, each needing its own degree, so
// the lowest it can sit is TempMin+3.
func TestEnforceCurve_TemperaturesStrictlyIncrease(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	c[3].Temp = 20
	l.EnforceCurve(&c, 3, PWMMin)

	if want := l.TempMin + 3; c[3].Temp != want {
		t.Errorf("dragged point temp = %d, want %d (leaves room for points 0-2)", c[3].Temp, want)
	}
	assertCurveValid(t, l, c, PWMMin)
}

// The mirror of the above: a point dragged off the right edge must leave room
// for the points after it.
func TestEnforceCurve_LeavesRoomAboveDraggedPoint(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	c[4].Temp = 999
	l.EnforceCurve(&c, 4, PWMMin)

	if want := l.TempMax - (CurvePoints - 1 - 4); c[4].Temp != want {
		t.Errorf("dragged point temp = %d, want %d (leaves room for points 5-7)", c[4].Temp, want)
	}
	assertCurveValid(t, l, c, PWMMin)
}

func TestEnforceCurve_PWMNeverDecreases(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	c[1].PWM = 240
	l.EnforceCurve(&c, 1, PWMMin)

	if c[1].PWM != 240 {
		t.Errorf("dragged point PWM = %d, want 240 preserved", c[1].PWM)
	}
	assertCurveValid(t, l, c, PWMMin)
}

func TestEnforceCurve_DraggedPointWinsOverNeighbours(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	c[5].PWM = 30
	l.EnforceCurve(&c, 5, PWMMin)

	if c[5].PWM != 30 {
		t.Errorf("dragged point PWM = %d, want 30 preserved", c[5].PWM)
	}
	assertCurveValid(t, l, c, PWMMin)
}

func TestEnforceCurve_ClampsOutOfRange(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	c[7].Temp = 500
	c[7].PWM = 9000
	l.EnforceCurve(&c, 7, PWMMin)

	if c[7].Temp != l.TempMax {
		t.Errorf("temp = %d, want clamped to %d", c[7].Temp, l.TempMax)
	}
	if c[7].PWM != PWMMax {
		t.Errorf("PWM = %d, want clamped to %d", c[7].PWM, PWMMax)
	}
	assertCurveValid(t, l, c, PWMMin)
}

// The floor is the whole reason the drawer cannot let a drag go low: the daemon
// rejects the entire curve if any point is under it while PL1 is high.
func TestEnforceCurve_HighTDPFloorLiftsEveryPoint(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve() // 7 of its 8 points sit below the floor
	l.EnforceCurve(&c, 0, l.HighTDPMinPWM)

	for i, p := range c {
		if p.PWM < l.HighTDPMinPWM {
			t.Errorf("point %d PWM = %d, below floor %d", i, p.PWM, l.HighTDPMinPWM)
		}
	}
	assertCurveValid(t, l, c, l.HighTDPMinPWM)
}

func TestEnforceCurve_DragBelowFloorIsLifted(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	l.EnforceCurve(&c, 0, l.HighTDPMinPWM) // start from a floored curve

	c[2].PWM = 10 // user drags a point to the bottom
	l.EnforceCurve(&c, 2, l.HighTDPMinPWM)

	if c[2].PWM < l.HighTDPMinPWM {
		t.Errorf("dragged point PWM = %d, want lifted to at least %d", c[2].PWM, l.HighTDPMinPWM)
	}
	assertCurveValid(t, l, c, l.HighTDPMinPWM)
}

func TestEnforceCurve_Idempotent(t *testing.T) {
	l := DefaultLimits()
	for _, floor := range []int{PWMMin, l.HighTDPMinPWM} {
		c := l.DefaultCurve()
		c[4].PWM = 12
		c[4].Temp = 33
		l.EnforceCurve(&c, 4, floor)
		once := c
		l.EnforceCurve(&c, 4, floor)
		if c != once {
			t.Errorf("floor %d: second pass changed the curve: %v then %v", floor, once, c)
		}
	}
}

func TestEnforceCurve_OutOfRangeIndexIsIgnored(t *testing.T) {
	l := DefaultLimits()
	c := l.DefaultCurve()
	before := c
	l.EnforceCurve(&c, -1, PWMMin)
	l.EnforceCurve(&c, CurvePoints, PWMMin)
	if c != before {
		t.Error("out-of-range index modified the curve")
	}
}

// Every index dragged to every extreme, on both devices, at every floor. This is
// the sweep that caught the original squash bug, and it is what proves the
// constraint logic is not tuned to the Z13's temperature range.
func TestEnforceCurve_AllIndicesAtBothExtremesOnEveryDevice(t *testing.T) {
	for _, l := range []Limits{DefaultLimits(), otherDevice()} {
		for _, floor := range []int{PWMMin, l.HighTDPMinPWM} {
			for idx := 0; idx < CurvePoints; idx++ {
				for _, temp := range []int{-100, 0, 20, 500} {
					c := l.DefaultCurve()
					c[idx].Temp = temp
					l.EnforceCurve(&c, idx, floor)
					assertCurveValid(t, l, c, floor)
				}
			}
		}
	}
}

// assertCurveValid checks every invariant the daemon and firmware require.
func assertCurveValid(t *testing.T, l Limits, c Curve, minPWM int) {
	t.Helper()
	for i, p := range c {
		if p.Temp < l.TempMin || p.Temp > l.TempMax {
			t.Errorf("%s: point %d temp = %d, outside [%d,%d]", l.Model, i, p.Temp, l.TempMin, l.TempMax)
		}
		if p.PWM < minPWM || p.PWM > PWMMax {
			t.Errorf("%s: point %d PWM = %d, outside [%d,%d]", l.Model, i, p.PWM, minPWM, PWMMax)
		}
		if i > 0 {
			if c[i].Temp <= c[i-1].Temp {
				t.Errorf("%s: temps not strictly increasing at %d: %d then %d", l.Model, i, c[i-1].Temp, c[i].Temp)
			}
			if c[i].PWM < c[i-1].PWM {
				t.Errorf("%s: PWM decreased at %d: %d then %d", l.Model, i, c[i-1].PWM, c[i].PWM)
			}
		}
	}
}

// assertLimitsUsable states what the rest of the drawer assumes about a Limits
// value: the TDP bounds are ordered, the temperature axis is wide enough for a
// curve, and the fan floor is a real PWM value. Anything violating these produces
// either a curve the daemon rejects or NaN coordinates in the editor.
func assertLimitsUsable(t *testing.T, l Limits) {
	t.Helper()
	if l.TDPMin >= l.TDPMaxSafe {
		t.Errorf("TDPMin %d not below TDPMaxSafe %d", l.TDPMin, l.TDPMaxSafe)
	}
	if l.TDPMaxSafe > l.TDPMaxForced {
		t.Errorf("TDPMaxSafe %d above TDPMaxForced %d", l.TDPMaxSafe, l.TDPMaxForced)
	}
	if got := l.TempMax - l.TempMin; got < CurvePoints-1 {
		t.Errorf("temperature range %d too narrow for %d points", got, CurvePoints)
	}
	if l.HighTDPMinPWM < PWMMin || l.HighTDPMinPWM > PWMMax {
		t.Errorf("HighTDPMinPWM %d outside [%d,%d]", l.HighTDPMinPWM, PWMMin, PWMMax)
	}
	if l.BasicSliderMax() > l.TDPMaxForced {
		t.Errorf("BasicSliderMax %d above TDPMaxForced %d", l.BasicSliderMax(), l.TDPMaxForced)
	}
	if len(l.StockProfilePPT) == 0 {
		t.Error("StockProfilePPT is empty")
	}
}

// TestSanitizedRepairsInconsistentLimits covers the values a daemon-served
// device description could carry that a zero check cannot catch. This is the path
// multi-device support opens: today Limits is a compiled-in constant, tomorrow it
// arrives over a socket from a daemon of unknown version.
func TestSanitizedRepairsInconsistentLimits(t *testing.T) {
	d := DefaultLimits()

	tests := []struct {
		name string
		in   Limits
	}{
		{"zero value", Limits{}},
		{"temp axis inverted", Limits{TempMin: 90, TempMax: 40}},
		{"temp axis collapsed", Limits{TempMin: 50, TempMax: 50}},
		{"temp axis too narrow for the curve", Limits{TempMin: 50, TempMax: 55}},
		{"temp axis exactly one degree short", Limits{TempMin: 40, TempMax: 40 + CurvePoints - 2}},
		{"safe max below the minimum", Limits{TDPMin: 60, TDPMaxSafe: 30, TDPMaxForced: 90}},
		{"safe max above the forced max", Limits{TDPMin: 5, TDPMaxSafe: 95, TDPMaxForced: 90}},
		{"min equals safe max", Limits{TDPMin: 50, TDPMaxSafe: 50, TDPMaxForced: 90}},
		{"fan floor above the hwmon maximum", Limits{HighTDPMinPWM: 999}},
		{"fan floor negative", Limits{HighTDPMinPWM: -5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Sanitized()
			assertLimitsUsable(t, got)
			// Whatever it repaired, the placeholder curve must still be valid — the
			// editor draws it before the daemon reports anything.
			assertCurveValid(t, got, got.DefaultCurve(), PWMMin)
		})
	}

	// A device with no fan floor keeps that: zero is legitimate there, and the
	// repair above must not mistake it for a missing value.
	t.Run("zero fan floor is preserved", func(t *testing.T) {
		if got := (Limits{HighTDPMinPWM: 0}).Sanitized(); got.HighTDPMinPWM != 0 {
			t.Errorf("HighTDPMinPWM = %d, want 0 preserved", got.HighTDPMinPWM)
		}
	})

	// A consistent description must pass through untouched, so the repair cannot
	// quietly overwrite a legitimate device with the Z13's numbers.
	t.Run("consistent limits are untouched", func(t *testing.T) {
		for _, l := range []Limits{d, otherDevice()} {
			got := l.Sanitized()
			if got.TDPMin != l.TDPMin || got.TDPMaxSafe != l.TDPMaxSafe ||
				got.TDPMaxForced != l.TDPMaxForced ||
				got.TempMin != l.TempMin || got.TempMax != l.TempMax ||
				got.HighTDPMinPWM != l.HighTDPMinPWM {
				t.Errorf("%s: Sanitized altered a consistent value: %+v -> %+v", l.Model, l, got)
			}
		}
	})
}

// TestSanitizedIsIdempotent — a repaired value must not repair further, or the
// result would depend on how many times it had been through.
func TestSanitizedIsIdempotent(t *testing.T) {
	for _, in := range []Limits{{}, {TempMin: 50, TempMax: 50}, {TDPMin: 60, TDPMaxSafe: 30}} {
		once := in.Sanitized()
		twice := once.Sanitized()
		if once.TDPMin != twice.TDPMin || once.TDPMaxSafe != twice.TDPMaxSafe ||
			once.TDPMaxForced != twice.TDPMaxForced ||
			once.TempMin != twice.TempMin || once.TempMax != twice.TempMax ||
			once.HighTDPMinPWM != twice.HighTDPMinPWM {
			t.Errorf("not idempotent: %+v -> %+v -> %+v", in, once, twice)
		}
	}
}

// TestEnforceCurveSurvivesASanitizedNarrowAxis is the failure this repair
// prevents, stated end to end: take the narrowest axis Sanitized will accept,
// push a point to each extreme, and require a valid curve every time.
func TestEnforceCurveOnNarrowestAcceptedAxis(t *testing.T) {
	l := Limits{TempMin: 40, TempMax: 40 + CurvePoints - 1}.Sanitized()
	assertLimitsUsable(t, l)

	for idx := 0; idx < CurvePoints; idx++ {
		for _, temp := range []int{-100, 0, l.TempMin, l.TempMax, 10000} {
			c := l.DefaultCurve()
			c[idx].Temp = temp
			l.EnforceCurve(&c, idx, PWMMin)
			assertCurveValid(t, l, c, PWMMin)
		}
	}
}
