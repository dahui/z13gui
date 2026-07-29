package colorconv

import (
	"fmt"
	"math"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{in: "FF6600", want: "FF6600", wantOK: true},
		{in: "ff6600", want: "FF6600", wantOK: true},
		{in: "#FF6600", want: "FF6600", wantOK: true},
		{in: "  #ff6600  ", want: "FF6600", wantOK: true},
		{in: "F60", want: "FF6600", wantOK: true}, // shorthand expands
		{in: "#f60", want: "FF6600", wantOK: true},
		{in: "000000", want: "000000", wantOK: true},
		{in: "FFFFFF", want: "FFFFFF", wantOK: true},

		// Rejected. Each of these used to yield pure black silently.
		{in: "", wantOK: false},
		{in: "#", wantOK: false},
		{in: "GGGGGG", wantOK: false},
		{in: "FF66", wantOK: false},
		{in: "FF66000", wantOK: false},
		{in: "FF 660", wantOK: false},
		{in: "0x1234", wantOK: false},
		{in: "-F0000", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.in), func(t *testing.T) {
			got, ok := Normalize(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("Normalize(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The regression this package exists for: an unparseable colour must be
// reportable as invalid, not silently indistinguishable from black. The drawer
// takes this value from daemon state, and a corrupt state file used to blacken the
// swatch and then get written back to the hardware on the next apply.
func TestHexToHSLRejectsGarbageInsteadOfReturningBlack(t *testing.T) {
	for _, bad := range []string{"", "nonsense", "#", "12345", "GGGGGG"} {
		h, s, l, ok := HexToHSL(bad)
		if ok {
			t.Errorf("HexToHSL(%q) ok = true, want false", bad)
		}
		if h != 0 || s != 0 || l != 0 {
			t.Errorf("HexToHSL(%q) = (%v,%v,%v), want zeroes alongside ok=false", bad, h, s, l)
		}
	}

	// Real black is valid and must be distinguishable from the failure above by
	// the ok flag alone.
	if _, _, _, ok := HexToHSL("000000"); !ok {
		t.Error("HexToHSL(000000) ok = false, want true — black is a colour")
	}
}

func TestHexToHSLKnownValues(t *testing.T) {
	tests := []struct {
		hex     string
		h, s, l float64
	}{
		{hex: "000000", h: 0, s: 0, l: 0},
		{hex: "FFFFFF", h: 0, s: 0, l: 100},
		{hex: "808080", h: 0, s: 0, l: 50.196},
		{hex: "FF0000", h: 0, s: 100, l: 50},
		{hex: "00FF00", h: 120, s: 100, l: 50},
		{hex: "0000FF", h: 240, s: 100, l: 50},
		{hex: "FFFF00", h: 60, s: 100, l: 50},
		{hex: "00FFFF", h: 180, s: 100, l: 50},
		{hex: "FF00FF", h: 300, s: 100, l: 50},
	}
	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			h, s, l, ok := HexToHSL(tt.hex)
			if !ok {
				t.Fatalf("HexToHSL(%s) ok = false", tt.hex)
			}
			const tol = 0.01
			if math.Abs(h-tt.h) > tol || math.Abs(s-tt.s) > tol || math.Abs(l-tt.l) > tol {
				t.Errorf("HexToHSL(%s) = (%.3f,%.3f,%.3f), want (%v,%v,%v)", tt.hex, h, s, l, tt.h, tt.s, tt.l)
			}
		})
	}
}

// Hue is meaningless without saturation, so it must come back as a definite 0
// rather than whatever the branch arithmetic happened to leave behind.
func TestHexToHSLGreysHaveZeroHueAndSaturation(t *testing.T) {
	for _, grey := range []string{"000000", "111111", "808080", "CCCCCC", "FFFFFF"} {
		h, s, _, ok := HexToHSL(grey)
		if !ok {
			t.Fatalf("HexToHSL(%s) ok = false", grey)
		}
		if h != 0 || s != 0 {
			t.Errorf("HexToHSL(%s) = h %v s %v, want both 0", grey, h, s)
		}
	}
}

func TestHexToHSLHueIsInRange(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				hex := fmt.Sprintf("%02X%02X%02X", r, g, b)
				h, s, l, ok := HexToHSL(hex)
				if !ok {
					t.Fatalf("HexToHSL(%s) ok = false", hex)
				}
				if h < 0 || h >= 360 {
					t.Errorf("HexToHSL(%s) h = %v, outside [0,360)", hex, h)
				}
				if s < 0 || s > 100 {
					t.Errorf("HexToHSL(%s) s = %v, outside [0,100]", hex, s)
				}
				if l < 0 || l > 100 {
					t.Errorf("HexToHSL(%s) l = %v, outside [0,100]", hex, l)
				}
			}
		}
	}
}

// The invariant that matters in the picker: opening the colour view converts hex
// to slider positions, and touching a slider converts back. A lossy round trip
// would drift the colour every time the view is opened.
func TestRoundTripHexToHSLAndBack(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				want := fmt.Sprintf("%02X%02X%02X", r, g, b)
				h, s, l, ok := HexToHSL(want)
				if !ok {
					t.Fatalf("HexToHSL(%s) ok = false", want)
				}
				if got := HSLToHex(h, s, l); got != want {
					t.Errorf("round trip %s -> (%.3f,%.3f,%.3f) -> %s", want, h, s, l, got)
				}
			}
		}
	}
}

func TestHSLToHexAlwaysProducesAValidColour(t *testing.T) {
	for h := -720.0; h <= 1080; h += 37 {
		for s := -50.0; s <= 150; s += 25 {
			for l := -50.0; l <= 150; l += 25 {
				got := HSLToHex(h, s, l)
				if norm, ok := Normalize(got); !ok || norm != got {
					t.Errorf("HSLToHex(%v,%v,%v) = %q, not a canonical colour", h, s, l, got)
				}
			}
		}
	}
}

// Slider values arrive unclamped from GTK and hue wraps, so out-of-range input
// must behave rather than produce something like "-1-1-1".
func TestHSLToHexClampsAndWraps(t *testing.T) {
	if got, want := HSLToHex(0, 100, 50), "FF0000"; got != want {
		t.Errorf("HSLToHex(0,100,50) = %s, want %s", got, want)
	}
	if got, want := HSLToHex(360, 100, 50), "FF0000"; got != want {
		t.Errorf("hue 360 = %s, want %s (wraps to 0)", got, want)
	}
	if got, want := HSLToHex(-360, 100, 50), "FF0000"; got != want {
		t.Errorf("hue -360 = %s, want %s (wraps to 0)", got, want)
	}
	if got, want := HSLToHex(720+120, 100, 50), "00FF00"; got != want {
		t.Errorf("hue 840 = %s, want %s (wraps to 120)", got, want)
	}
	if got, want := HSLToHex(0, 999, 50), "FF0000"; got != want {
		t.Errorf("saturation 999 = %s, want %s (clamped to 100)", got, want)
	}
	if got, want := HSLToHex(0, 100, 999), "FFFFFF"; got != want {
		t.Errorf("lightness 999 = %s, want %s (clamped to 100)", got, want)
	}
	if got, want := HSLToHex(0, 100, -999), "000000"; got != want {
		t.Errorf("lightness -999 = %s, want %s (clamped to 0)", got, want)
	}
}
