// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

// Package lighting holds the drawer's RGB lighting rules: which mode a daemon
// state represents, which controls that mode needs, and what to fall back to when
// state is missing.
//
// Separate from internal/gui because that package needs CGO and GTK4 headers and
// cannot be unit tested. These are decisions about daemon state, not widgets.
package lighting

import "github.com/dahui/z13ctl/api"

// Defaults used when daemon state is unavailable — before the first sync, or when
// the daemon is not running.
const (
	DefaultColor1     = "FF0000"
	DefaultColor2     = "000000"
	DefaultMode       = "static"
	DefaultSpeed      = "normal"
	DefaultBrightness = 3

	// ModeOff is the drawer's pseudo-mode for "lighting disabled". The daemon
	// represents this as Enabled=false, and the drawer needs a selectable button
	// for it.
	//
	// A per-zone off, the only kind the drawer issues, keeps the zone's effect
	// and only clears Enabled on a daemon newer than z13ctl 1.4.2 (z13ctl PR
	// #31). z13ctl 1.4.2 and earlier instead store LightingState{Enabled: false}
	// with mode, colours, speed and brightness all zeroed, and a state file
	// written by one still carries such entries until the zone is next set. So
	// every field read out of a disabled state still needs a fallback — see
	// ResolveBrightness for what happens when one does not have it.
	ModeOff = "off"
)

// Controls says which of the lighting sub-controls apply to a mode. A mode that
// does not animate has no speed; one that ignores colour has no swatches.
type Controls struct {
	Color1     bool
	Color2     bool
	Speed      bool
	Brightness bool
}

// modeControls is the per-mode table. Unknown modes are handled by ControlsFor.
var modeControls = map[string]Controls{
	"static":  {Color1: true, Brightness: true},
	"breathe": {Color1: true, Color2: true, Speed: true, Brightness: true},
	"cycle":   {Speed: true, Brightness: true},
	"rainbow": {Speed: true, Brightness: true},
	"strobe":  {Color1: true, Speed: true, Brightness: true},
	ModeOff:   {},
}

// ControlsFor returns the controls a mode needs.
//
// An unrecognised mode shows everything. A newer daemon may know modes this build
// does not, and revealing all the controls lets the user still operate them;
// hiding them would make the mode look broken.
func ControlsFor(mode string) Controls {
	if c, ok := modeControls[mode]; ok {
		return c
	}
	return Controls{Color1: true, Color2: true, Speed: true, Brightness: true}
}

// KnownMode reports whether mode is one this build has a control layout for.
func KnownMode(mode string) bool {
	_, ok := modeControls[mode]
	return ok
}

// ResolveMode returns the mode button the drawer should select for a lighting
// state.
//
// Disabled lighting selects ModeOff regardless of any mode the daemon still has
// recorded: showing "breathe" as active while the keyboard is dark would be a lie.
//
// An enabled state with no mode falls back to the default rather than selecting
// nothing: the daemon can legitimately store a partial per-zone entry, which is
// what made zone lighting come back blank after a reboot.
func ResolveMode(ls api.LightingState) string {
	if !ls.Enabled {
		return ModeOff
	}
	if ls.Mode == "" {
		return DefaultMode
	}
	return ls.Mode
}

// ResolveSpeed returns the speed to select, falling back when unset.
func ResolveSpeed(ls api.LightingState) string {
	if ls.Speed == "" {
		return DefaultSpeed
	}
	return ls.Speed
}

// ResolveBrightness returns the brightness the slider should show.
//
// A disabled state reports its stored brightness when that is above zero, and
// the default otherwise. A daemon newer than z13ctl 1.4.2 keeps a zone's
// brightness when it is turned off, so re-enabling brings the zone back at the
// level it had. z13ctl 1.4.2 and earlier replace the whole entry with
// LightingState{Enabled: false} — every other field zeroed — so the stored value
// is 0, and 0 is the hardware's "off" level, not merely a dim one. The drawer
// cannot tell which daemon wrote an entry, and a zone switched off at brightness
// 0 stores a disabled 0 on both, so a stored 0 always falls back to the default.
//
// Without this, turning a zone off and then back on left the keyboard dark: the
// slider adopted the zero, the next apply sent brightness 0, and the daemon
// dutifully set the backlight to off while reporting success. The mode button lit
// up and nothing else happened.
//
// A zero brightness on an *enabled* state is passed through, since that is a
// setting the user can deliberately choose with the slider, and second-guessing it
// would misreport the hardware. This is the same partial-state problem ResolveMode
// and ResolveSpeed already guard against; brightness was simply missed.
func ResolveBrightness(ls api.LightingState) int {
	if !ls.Enabled {
		if ls.Brightness > 0 {
			return ls.Brightness
		}
		return DefaultBrightness
	}
	return ls.Brightness
}

// StateForZone picks the lighting state to display for a zone, preferring the
// per-device entry and falling back to the global one.
//
// Returns the zero state when nothing is available, which ResolveMode reads as
// disabled — the correct thing to show when the daemon has told us nothing.
func StateForZone(s *api.State, zone string) api.LightingState {
	if s == nil {
		return api.LightingState{}
	}
	if dev, ok := s.Devices[zone]; ok {
		return dev
	}
	return s.Lighting
}
