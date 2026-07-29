package power

import (
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestNeedsAdvanced(t *testing.T) {
	tests := []struct {
		name string
		tdp  api.TDPState
		want bool
	}{
		{
			// The regression this function exists for: PL1 above the basic
			// slider's ceiling used to clamp to 70 and report "70 W" while the
			// hardware ran at 80 W, and a save would then send 70.
			name: "sustained above basic ceiling",
			tdp:  api.TDPState{PL1SPL: 80, PL2SPPT: 80, FPPT: 80},
			want: true,
		},
		{
			name: "exactly at the basic ceiling is representable",
			tdp:  api.TDPState{PL1SPL: 70, PL2SPPT: 70, FPPT: 70},
			want: false,
		},
		{
			name: "one over the basic ceiling is not",
			tdp:  api.TDPState{PL1SPL: 71, PL2SPPT: 71, FPPT: 71},
			want: true,
		},
		{
			// Basic mode applies one value to all three, so differing limits
			// cannot be shown even when every value is inside the basic range.
			name: "limits differ below the ceiling",
			tdp:  api.TDPState{PL1SPL: 52, PL2SPPT: 71, FPPT: 70},
			want: true,
		},
		{
			name: "only PL2 differs",
			tdp:  api.TDPState{PL1SPL: 50, PL2SPPT: 60, FPPT: 50},
			want: true,
		},
		{
			name: "only PL3 differs",
			tdp:  api.TDPState{PL1SPL: 50, PL2SPPT: 50, FPPT: 60},
			want: true,
		},
		{
			// What a basic save round-trips as: the daemon defaults the blank
			// PL fields to the single value, so this must stay in basic.
			name: "equal triple from a basic save",
			tdp:  api.TDPState{PL1SPL: 45, PL2SPPT: 45, FPPT: 45},
			want: false,
		},
		{
			// APUSPPT/PlatformSPPT are daemon bookkeeping mirrored from PL2 and
			// are not shown in the drawer, so they must not force advanced.
			name: "mirrored APU fields are ignored",
			tdp:  api.TDPState{PL1SPL: 50, PL2SPPT: 50, FPPT: 50, APUSPPT: 70, PlatformSPPT: 70},
			want: false,
		},
		{
			name: "zero value is representable",
			tdp:  api.TDPState{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsAdvanced(tt.tdp); got != tt.want {
				t.Errorf("NeedsAdvanced(%+v) = %v, want %v", tt.tdp, got, tt.want)
			}
		})
	}
}

func TestFanFloorPWM(t *testing.T) {
	tests := []struct {
		pl1  int
		want int
	}{
		{pl1: 0, want: PWMMin},
		{pl1: 50, want: PWMMin},
		{pl1: TDPMaxSafe, want: PWMMin},            // at the threshold, still unconstrained
		{pl1: TDPMaxSafe + 1, want: HighTDPMinPWM}, // one over, floor applies
		{pl1: 80, want: HighTDPMinPWM},
		{pl1: TDPMaxAdvanced, want: HighTDPMinPWM},
	}
	for _, tt := range tests {
		if got := FanFloorPWM(tt.pl1); got != tt.want {
			t.Errorf("FanFloorPWM(%d) = %d, want %d", tt.pl1, got, tt.want)
		}
	}
}

func TestForceRequired(t *testing.T) {
	if ForceRequired(TDPMaxSafe) {
		t.Errorf("ForceRequired(%d) = true, want false at the threshold", TDPMaxSafe)
	}
	if !ForceRequired(TDPMaxSafe + 1) {
		t.Errorf("ForceRequired(%d) = false, want true above the threshold", TDPMaxSafe+1)
	}
}

func TestDefaultCurveIsValid(t *testing.T) {
	c := DefaultCurve()
	assertCurveValid(t, c, PWMMin)
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
func TestEnforceConstraints_TemperaturesStrictlyIncrease(t *testing.T) {
	c := DefaultCurve()
	c[3].Temp = 20
	c.EnforceConstraints(3, PWMMin)

	if want := TempMin + 3; c[3].Temp != want {
		t.Errorf("dragged point temp = %d, want %d (leaves room for points 0-2)", c[3].Temp, want)
	}
	assertCurveValid(t, c, PWMMin)
}

// The mirror of the above: a point dragged off the right edge must leave room
// for the points after it.
func TestEnforceConstraints_LeavesRoomAboveDraggedPoint(t *testing.T) {
	c := DefaultCurve()
	c[4].Temp = 999
	c.EnforceConstraints(4, PWMMin)

	if want := TempMax - (CurvePoints - 1 - 4); c[4].Temp != want {
		t.Errorf("dragged point temp = %d, want %d (leaves room for points 5-7)", c[4].Temp, want)
	}
	assertCurveValid(t, c, PWMMin)
}

// Every index dragged to both extremes must still yield a valid curve — this is
// the case that caught the original squash bug.
func TestEnforceConstraints_AllIndicesAtBothExtremes(t *testing.T) {
	for _, floor := range []int{PWMMin, HighTDPMinPWM} {
		for idx := 0; idx < CurvePoints; idx++ {
			for _, temp := range []int{-100, 0, 20, 500} {
				c := DefaultCurve()
				c[idx].Temp = temp
				c.EnforceConstraints(idx, floor)
				assertCurveValid(t, c, floor)
			}
		}
	}
}

func TestEnforceConstraints_PWMNeverDecreases(t *testing.T) {
	c := DefaultCurve()
	// Raise an early point above several later ones.
	c[1].PWM = 240
	c.EnforceConstraints(1, PWMMin)

	if c[1].PWM != 240 {
		t.Errorf("dragged point PWM = %d, want 240 preserved", c[1].PWM)
	}
	assertCurveValid(t, c, PWMMin)
}

func TestEnforceConstraints_DraggedPointWinsOverNeighbours(t *testing.T) {
	c := DefaultCurve()
	// The moved point must keep its value; neighbours give way around it.
	c[5].PWM = 30
	c.EnforceConstraints(5, PWMMin)

	if c[5].PWM != 30 {
		t.Errorf("dragged point PWM = %d, want 30 preserved", c[5].PWM)
	}
	assertCurveValid(t, c, PWMMin)
}

func TestEnforceConstraints_ClampsOutOfRange(t *testing.T) {
	c := DefaultCurve()
	c[7].Temp = 500
	c[7].PWM = 9000
	c.EnforceConstraints(7, PWMMin)

	if c[7].Temp != TempMax {
		t.Errorf("temp = %d, want clamped to %d", c[7].Temp, TempMax)
	}
	if c[7].PWM != PWMMax {
		t.Errorf("PWM = %d, want clamped to %d", c[7].PWM, PWMMax)
	}
	assertCurveValid(t, c, PWMMin)
}

// The floor is the whole reason the drawer cannot let a drag go low: the daemon
// rejects the entire curve if any point is under it while PL1 is high.
func TestEnforceConstraints_HighTDPFloorLiftsEveryPoint(t *testing.T) {
	c := DefaultCurve() // 7 of its 8 points sit below the floor
	c.EnforceConstraints(0, HighTDPMinPWM)

	for i, p := range c {
		if p.PWM < HighTDPMinPWM {
			t.Errorf("point %d PWM = %d, below floor %d", i, p.PWM, HighTDPMinPWM)
		}
	}
	assertCurveValid(t, c, HighTDPMinPWM)
}

func TestEnforceConstraints_DragBelowFloorIsLifted(t *testing.T) {
	c := DefaultCurve()
	c.EnforceConstraints(0, HighTDPMinPWM) // start from a floored curve

	c[2].PWM = 10 // user drags a point to the bottom
	c.EnforceConstraints(2, HighTDPMinPWM)

	if c[2].PWM < HighTDPMinPWM {
		t.Errorf("dragged point PWM = %d, want lifted to at least %d", c[2].PWM, HighTDPMinPWM)
	}
	assertCurveValid(t, c, HighTDPMinPWM)
}

// A curve saved while the floor was off gets re-synced once a high TDP is
// applied; it must come back valid rather than being sent and rejected.
func TestEnforceConstraints_LiftsCurveSavedBeforeFloorApplied(t *testing.T) {
	c := DefaultCurve()
	c.EnforceConstraints(0, HighTDPMinPWM)
	assertCurveValid(t, c, HighTDPMinPWM)
}

func TestEnforceConstraints_Idempotent(t *testing.T) {
	for _, floor := range []int{PWMMin, HighTDPMinPWM} {
		c := DefaultCurve()
		c[4].PWM = 12
		c[4].Temp = 33
		c.EnforceConstraints(4, floor)
		once := c
		c.EnforceConstraints(4, floor)
		if c != once {
			t.Errorf("floor %d: second pass changed the curve: %v then %v", floor, once, c)
		}
	}
}

func TestEnforceConstraints_OutOfRangeIndexIsIgnored(t *testing.T) {
	c := DefaultCurve()
	before := c
	c.EnforceConstraints(-1, PWMMin)
	c.EnforceConstraints(CurvePoints, PWMMin)
	if c != before {
		t.Error("out-of-range index modified the curve")
	}
}

// assertCurveValid checks every invariant the daemon and firmware require.
func assertCurveValid(t *testing.T, c Curve, minPWM int) {
	t.Helper()
	for i, p := range c {
		if p.Temp < TempMin || p.Temp > TempMax {
			t.Errorf("point %d temp = %d, outside [%d,%d]", i, p.Temp, TempMin, TempMax)
		}
		if p.PWM < minPWM || p.PWM > PWMMax {
			t.Errorf("point %d PWM = %d, outside [%d,%d]", i, p.PWM, minPWM, PWMMax)
		}
		if i > 0 {
			if c[i].Temp <= c[i-1].Temp {
				t.Errorf("temps not strictly increasing at %d: %d then %d", i, c[i-1].Temp, c[i].Temp)
			}
			if c[i].PWM < c[i-1].PWM {
				t.Errorf("PWM decreased at %d: %d then %d", i, c[i-1].PWM, c[i].PWM)
			}
		}
	}
}
