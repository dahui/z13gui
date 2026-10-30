// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

// Package profileinfo builds user-facing descriptions of z13ctl power profiles.
package profileinfo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dahui/z13ctl/api"
	"github.com/dahui/z13gui/internal/power"
)

var (
	stockProfiles = []string{"quiet", "balanced", "performance"}
	stockPPT      = power.DefaultLimits().StockProfilePPT
)

// TDP returns the power limits for profile, preferring the live daemon reading
// for the active profile and the stored settings for custom profiles.
func TDP(profile string, state *api.State) (api.TDPState, bool) {
	if state != nil {
		if state.Profile == profile && state.TDP != nil {
			return *state.TDP, true
		}
		if p, ok := state.CustomProfiles[profile]; ok && p.TDP != nil {
			return *p.TDP, true
		}
	}
	t, ok := stockPPT[profile]
	return t, ok
}

// Describe returns a one-line description of a profile's power limit and fan
// behaviour.
func Describe(profile string, state *api.State) string {
	tdp, ok := TDP(profile, state)
	powerText := "Custom TDP"
	if ok {
		powerText = fmt.Sprintf("%d/%d/%d W", tdp.PL1SPL, tdp.PL2SPPT, tdp.FPPT)
	}

	switch profile {
	case "quiet":
		return powerText + ". Quiet fan, lowest noise."
	case "balanced":
		return powerText + ". Balanced fan, moderate noise."
	case "performance":
		return powerText + ". Performance fan, highest noise."
	}

	if hasCustomCurve(profile, state) {
		return powerText + ". Custom fan curve."
	}
	return powerText + ". Auto fan curve."
}

// Tooltip returns a Pango-markup list of every known profile.
func Tooltip(state *api.State) string {
	names := append([]string{}, stockProfiles...)
	seen := map[string]bool{
		"quiet": true, "balanced": true, "performance": true,
	}
	var custom []string
	if state != nil {
		for name := range state.CustomProfiles {
			if !seen[name] {
				seen[name] = true
				custom = append(custom, name)
			}
		}
	}
	sort.Strings(custom)
	names = append(names, custom...)
	if !seen["custom"] {
		names = append(names, "custom")
	}

	var b strings.Builder
	b.WriteString("<b>Power limits (PL1/PL2/PL3)</b>")
	for _, name := range names {
		fmt.Fprintf(&b, "\n<b>%s</b>: %s", Title(name), Describe(name, state))
	}
	return b.String()
}

// Title capitalizes a profile name for display.
func Title(name string) string {
	if name == "" {
		return "Unknown"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func hasCustomCurve(profile string, state *api.State) bool {
	if state == nil {
		return false
	}
	if state.Profile == profile && state.FanCurve != nil && state.FanCurve.Mode == 1 {
		return true
	}
	if p, ok := state.CustomProfiles[profile]; ok && p.FanCurve != nil && p.FanCurve.Mode == 1 {
		return true
	}
	return false
}
