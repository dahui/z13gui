// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

package profileinfo

import (
	"strings"
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestDescribeStock(t *testing.T) {
	cases := []struct {
		profile string
		want    string
	}{
		{"quiet", "40/55/55 W. Quiet fan, lowest noise."},
		{"balanced", "52/71/70 W. Balanced fan, moderate noise."},
		{"performance", "70/86/86 W. Performance fan, highest noise."},
	}
	for _, tc := range cases {
		if got := Describe(tc.profile, nil); got != tc.want {
			t.Errorf("Describe(%q) = %q, want %q", tc.profile, got, tc.want)
		}
	}
}

func TestDescribeActiveUsesLiveTDP(t *testing.T) {
	state := &api.State{
		Profile: "quiet",
		TDP:     &api.TDPState{PL1SPL: 41, PL2SPPT: 56, FPPT: 57},
	}
	if got, want := Describe("quiet", state), "41/56/57 W. Quiet fan, lowest noise."; got != want {
		t.Errorf("Describe(quiet) = %q, want %q", got, want)
	}
}

func TestDescribeCustom(t *testing.T) {
	state := &api.State{
		Profile: "custom",
		TDP:     &api.TDPState{PL1SPL: 30, PL2SPPT: 32, FPPT: 45},
		CustomProfiles: map[string]api.CustomProfile{
			"custom": {Name: "custom", TDP: &api.TDPState{PL1SPL: 30, PL2SPPT: 32, FPPT: 45}},
		},
	}
	if got, want := Describe("custom", state), "30/32/45 W. Auto fan curve."; got != want {
		t.Errorf("Describe(custom) = %q, want %q", got, want)
	}
}

func TestDescribeCustomCurve(t *testing.T) {
	state := &api.State{
		Profile:  "custom",
		FanCurve: &api.FanCurveState{Mode: 1},
		CustomProfiles: map[string]api.CustomProfile{
			"custom": {
				Name:     "custom",
				TDP:      &api.TDPState{PL1SPL: 30, PL2SPPT: 32, FPPT: 45},
				FanCurve: &api.FanCurveState{Mode: 1},
			},
		},
	}
	if got, want := Describe("custom", state), "30/32/45 W. Custom fan curve."; got != want {
		t.Errorf("Describe(custom) = %q, want %q", got, want)
	}
}

func TestTooltipListsProfiles(t *testing.T) {
	state := &api.State{
		Profile: "custom",
		TDP:     &api.TDPState{PL1SPL: 30, PL2SPPT: 32, FPPT: 45},
		CustomProfiles: map[string]api.CustomProfile{
			"custom": {Name: "custom", TDP: &api.TDPState{PL1SPL: 30, PL2SPPT: 32, FPPT: 45}},
		},
	}
	got := Tooltip(state)
	for _, want := range []string{
		"Quiet", "40/55/55 W",
		"Balanced", "52/71/70 W",
		"Performance", "70/86/86 W",
		"Custom", "30/32/45 W",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Tooltip missing %q:\n%s", want, got)
		}
	}
}
