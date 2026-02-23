package gui

// sync.go — daemon state synchronization and API communication.

import (
	"log/slog"
	"time"

	"github.com/dahui/z13ctl/api"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// modeVis defines which subsections are visible for a given lighting mode.
type modeVis struct{ color1, color2, speed bool }

// modeVisMap maps lighting mode names to their subsection visibility.
var modeVisMap = map[string]modeVis{
	"static":  {true, false, false},
	"breathe": {true, true, true},
	"cycle":   {false, false, true},
	"rainbow": {false, false, true},
	"strobe":  {true, false, true},
	"off":     {false, false, false},
}

// syncModeVis shows/hides color and speed sections based on the active mode.
// Safe to call at any time (including during sync).
func (w *Window) syncModeVis() {
	mode := "static"
	for m, btn := range w.modeButtons {
		if btn.Active() {
			mode = m
			break
		}
	}
	v, ok := modeVisMap[mode]
	if !ok {
		v = modeVis{true, true, true}
	}
	if w.color1Box != nil {
		w.color1Box.SetVisible(v.color1)
	}
	if w.color2Box != nil {
		w.color2Box.SetVisible(v.color2)
	}
	if w.speedBox != nil {
		w.speedBox.SetVisible(v.speed)
	}
}

// syncState updates all widgets from the current daemon state.
// Sets syncing=true to suppress signal handlers from firing sendApply.
func (w *Window) syncState() {
	if w.state == nil {
		return
	}
	w.syncing = true
	defer func() { w.syncing = false }()
	w.syncLightingSection()
	w.syncProfile()
	w.syncBattery()
	w.syncOverdrive()
	w.syncBootSound()
}

// syncLightingSection updates mode, colors, speed, and brightness from the
// daemon state for the active device tab.
func (w *Window) syncLightingSection() {
	prev := w.syncing
	w.syncing = true
	defer func() { w.syncing = prev }()

	var ls api.LightingState
	if w.state != nil {
		if dev, ok := w.state.Devices[w.tab]; ok {
			ls = dev
		} else {
			ls = w.state.Lighting
		}
	}
	if btn, ok := w.modeButtons[ls.Mode]; ok {
		btn.SetActive(true)
	}
	if w.color1 != nil && ls.Color != "" {
		w.color1.SetHex(ls.Color)
	}
	if w.color2 != nil && ls.Color2 != "" {
		w.color2.SetHex(ls.Color2)
	}
	w.updateSwatches()
	if btn, ok := w.speedBtns[ls.Speed]; ok {
		btn.SetActive(true)
	}
	if w.brightScale != nil {
		w.brightScale.SetValue(float64(ls.Brightness))
	}
	w.syncModeVis()
}

// syncProfile sets the profile dropdown to match the daemon state.
func (w *Window) syncProfile() {
	if w.state == nil || w.state.Profile == "" || w.profileDrop == nil {
		return
	}
	for i, p := range profiles {
		if p == w.state.Profile {
			w.profileDrop.SetSelected(uint(i))
			break
		}
	}
}

// syncBattery sets the battery limit scale to match the daemon state.
func (w *Window) syncBattery() {
	if w.state == nil || w.state.Battery == 0 || w.battScale == nil {
		return
	}
	w.battScale.SetValue(float64(w.state.Battery))
}

// queueApply debounces rapid API calls from continuous inputs (color wheel,
// sliders). Discrete inputs (mode buttons, speed buttons, preset clicks)
// call sendApply directly.
func (w *Window) queueApply() {
	if w.syncing {
		return
	}
	if w.applyTimer != nil {
		w.applyTimer.Stop()
	}
	w.applyTimer = time.AfterFunc(150*time.Millisecond, func() {
		glib.IdleAdd(func() bool {
			w.sendApply()
			return false
		})
	})
}

// sendApply sends the current lighting state to the daemon. Guarded by
// w.syncing to prevent sending defaults during widget initialization.
func (w *Window) sendApply() {
	if w.syncing {
		return
	}
	color1 := "FF0000"
	if w.color1 != nil {
		color1 = w.color1.hex
	}
	color2 := "000000"
	if w.color2 != nil {
		color2 = w.color2.hex
	}

	mode := "static"
	for m, btn := range w.modeButtons {
		if btn.Active() {
			mode = m
			break
		}
	}

	speed := "normal"
	for s, btn := range w.speedBtns {
		if btn.Active() {
			speed = s
			break
		}
	}

	brightness := 3
	if w.brightScale != nil {
		brightness = int(w.brightScale.Value())
	}

	// "off" is a UI-only pseudo-mode: send static at brightness 0.
	if mode == "off" {
		mode = "static"
		brightness = 0
	}

	slog.Debug("apply", "device", w.tab, "mode", mode, "brightness", brightness)
	if _, err := api.SendApply(w.tab, color1, color2, mode, speed, brightness); err != nil {
		slog.Warn("apply failed", "err", err)
	}
}

// sendProfileSet sends a profile change to the daemon.
func (w *Window) sendProfileSet(prof string) {
	if _, err := api.SendProfileSet(prof); err != nil {
		slog.Warn("profile set failed", "profile", prof, "err", err)
	}
}

// initBatteryDebounce sets up debounced battery limit changes on the given scale.
func (w *Window) initBatteryDebounce(sc *gtk.Scale) {
	var debounce *time.Timer
	sc.ConnectValueChanged(func() {
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(200*time.Millisecond, func() {
			glib.IdleAdd(func() bool {
				if _, err := api.SendBatteryLimitSet(int(sc.Value())); err != nil {
					slog.Warn("battery limit set failed", "err", err)
				}
				return false
			})
		})
	})
}

// syncOverdrive sets the overdrive switch to match the daemon state.
func (w *Window) syncOverdrive() {
	if w.state == nil || w.overdriveSwitch == nil {
		return
	}
	w.overdriveSwitch.SetActive(w.state.PanelOverdrive != 0)
}

// syncBootSound sets the boot sound switch to match the daemon state.
func (w *Window) syncBootSound() {
	if w.state == nil || w.bootSoundSwitch == nil {
		return
	}
	w.bootSoundSwitch.SetActive(w.state.BootSound != 0)
}

// sendOverdriveSet sends a panel overdrive change to the daemon.
func (w *Window) sendOverdriveSet(value int) {
	if _, err := api.SendPanelOverdriveSet(value); err != nil {
		slog.Warn("panel overdrive set failed", "value", value, "err", err)
	}
}

// sendBootSoundSet sends a boot sound change to the daemon.
func (w *Window) sendBootSoundSet(value int) {
	if _, err := api.SendBootSoundSet(value); err != nil {
		slog.Warn("boot sound set failed", "value", value, "err", err)
	}
}
