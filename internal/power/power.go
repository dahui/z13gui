// Package power holds the TDP and fan-curve rules the drawer needs in order to
// avoid offering the user a state the z13ctl daemon would refuse.
//
// It exists to be testable. The GTK code in internal/gui cannot be unit tested
// without CGO and GTK4 headers, so anything here is pure Go operating on plain
// values — no widgets, no daemon calls. internal/gui holds the widgets and
// delegates every decision to this package.
//
// The limits mirror z13ctl's internal/cli constants (cli.TDPMin, cli.TDPMaxSafe,
// cli.TDPMaxForced, cli.HighTDPMinPWM), which are not exported through the api
// module and therefore have to be duplicated. If they ever disagree the daemon
// wins: it validates against hardware, and these only exist so the UI does not
// present an option that will be rejected.
package power

import (
	"fmt"
	"strings"

	"github.com/dahui/z13ctl/api"
)

// TDP limits in watts.
const (
	TDPMin = 5 // absolute minimum

	// TDPMaxBasic is the ceiling of the drawer's single-slider basic view.
	TDPMaxBasic = 70

	// TDPMaxSafe is the maximum sustained limit the daemon accepts without the
	// force flag. It is also the threshold above which the fan floor applies.
	TDPMaxSafe = 75

	// TDPMaxAdvanced is the absolute hardware maximum (force required above
	// TDPMaxSafe).
	TDPMaxAdvanced = 93
)

// Fan curve limits.
const (
	// PWMMin and PWMMax bound a fan curve point's duty cycle.
	PWMMin = 0
	PWMMax = 255

	// TempMin and TempMax bound a fan curve point's temperature in Celsius.
	TempMin = 35
	TempMax = 105

	// HighTDPMinPWM is the minimum duty cycle (80%) the daemon accepts for any
	// curve point while the sustained limit exceeds TDPMaxSafe. Below it,
	// z13ctl rejects the whole curve.
	HighTDPMinPWM = 204

	// CurvePoints is the number of points in a fan curve; the firmware takes
	// exactly this many.
	CurvePoints = 8
)

// NeedsAdvanced reports whether a TDP state can only be shown accurately in the
// drawer's advanced view.
//
// Basic mode is a single slider that applies one value to all three power limits
// and stops at TDPMaxBasic, so it cannot represent either a sustained limit above
// that ceiling or a state where the three limits differ. Showing such a state in
// basic mode would clamp the slider and misreport the hardware — and worse, a
// subsequent save would send the clamped value and quietly lower the limit.
//
// A basic save round-trips as PL1 == PL2 == FPPT, because the daemon defaults the
// blank PL fields to the single value, so an equal triple never trips this.
func NeedsAdvanced(t api.TDPState) bool {
	if t.PL1SPL > TDPMaxBasic {
		return true
	}
	return t.PL1SPL != t.PL2SPPT || t.PL2SPPT != t.FPPT
}

// FanFloorPWM returns the minimum fan PWM the daemon will accept for a curve
// point given the applied sustained limit, or PWMMin when unconstrained.
//
// While pl1 is above TDPMaxSafe the daemon rejects any curve containing a point
// below HighTDPMinPWM, and refuses a fan reset outright — firmware auto has no
// floor at all, so releasing the fans there would remove the very protection the
// power limit requires. Resetting the TDP is the way back out.
func FanFloorPWM(pl1 int) int {
	if pl1 <= TDPMaxSafe {
		return PWMMin
	}
	return HighTDPMinPWM
}

// ForceRequired reports whether a TDP request needs the force flag, which the
// daemon demands for a sustained limit above TDPMaxSafe.
func ForceRequired(pl1 int) bool {
	return pl1 > TDPMaxSafe
}

// Curve is an 8-point fan curve, ordered by ascending temperature.
type Curve [CurvePoints]api.FanCurvePoint

// DefaultCurve returns the curve used before the daemon reports one.
func DefaultCurve() Curve {
	return Curve{
		{Temp: 35, PWM: 0},
		{Temp: 45, PWM: 25},
		{Temp: 50, PWM: 50},
		{Temp: 60, PWM: 80},
		{Temp: 70, PWM: 120},
		{Temp: 80, PWM: 170},
		{Temp: 90, PWM: 220},
		{Temp: 100, PWM: 255},
	}
}

// String renders the curve in the daemon's "temp:pwm,temp:pwm,..." wire format.
func (c Curve) String() string {
	parts := make([]string, 0, len(c))
	for _, p := range c {
		parts = append(parts, fmt.Sprintf("%d:%d", p.Temp, p.PWM))
	}
	return strings.Join(parts, ",")
}

// EnforceConstraints repairs the curve after point idx has been moved, so that
// it always satisfies what the firmware and daemon require:
//
//   - temperatures strictly increase
//   - PWM never decreases
//   - every point sits within [minPWM, PWMMax] and [TempMin, TempMax]
//
// minPWM comes from FanFloorPWM. Passing the floor here rather than reading it
// internally keeps the rule in one place and makes the clamping directly
// testable at both floor settings.
//
// The moved point is clamped first so it wins over its neighbours, then the
// change cascades outward in both directions, then a final pass re-clamps
// everything — cascading can push a neighbour past a bound.
//
// The moved point's temperature is clamped into a range that leaves room for the
// points on either side: each of the idx points below it needs at least one
// degree, as does each of the points above. Clamping it to the raw TempMin
// instead would push its left-hand neighbours below the minimum, and the final
// clamp would then pile them all onto TempMin — producing duplicate temperatures
// that are not strictly increasing.
func (c *Curve) EnforceConstraints(idx, minPWM int) {
	if idx < 0 || idx >= len(c) {
		return
	}
	clamp := func(p *api.FanCurvePoint) {
		if p.Temp < TempMin {
			p.Temp = TempMin
		}
		if p.Temp > TempMax {
			p.Temp = TempMax
		}
		if p.PWM < minPWM {
			p.PWM = minPWM
		}
		if p.PWM > PWMMax {
			p.PWM = PWMMax
		}
	}

	clamp(&c[idx])
	if lo := TempMin + idx; c[idx].Temp < lo {
		c[idx].Temp = lo
	}
	if hi := TempMax - (len(c) - 1 - idx); c[idx].Temp > hi {
		c[idx].Temp = hi
	}

	// Temperatures must strictly increase.
	for i := idx + 1; i < len(c); i++ {
		if c[i].Temp <= c[i-1].Temp {
			c[i].Temp = c[i-1].Temp + 1
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if c[i].Temp >= c[i+1].Temp {
			c[i].Temp = c[i+1].Temp - 1
		}
	}

	// PWM must not decrease.
	for i := idx + 1; i < len(c); i++ {
		if c[i].PWM < c[i-1].PWM {
			c[i].PWM = c[i-1].PWM
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if c[i].PWM > c[i+1].PWM {
			c[i].PWM = c[i+1].PWM
		}
	}

	for i := range c {
		clamp(&c[i])
	}
}
