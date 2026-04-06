package gui

// sync.go — daemon state synchronization and API communication.

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dahui/z13ctl/api"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Defaults used when widget state is unavailable (e.g. before first sync).
const (
	defaultColor1     = "FF0000"
	defaultColor2     = "000000"
	defaultMode       = "static"
	defaultSpeed      = "normal"
	defaultBrightness = 3
)

// modeVis defines which subsections are visible for a given lighting mode.
type modeVis struct{ color1, color2, speed, brightness bool }

// modeVisMap maps lighting mode names to their subsection visibility.
var modeVisMap = map[string]modeVis{
	"static":  {true, false, false, true},
	"breathe": {true, true, true, true},
	"cycle":   {false, false, true, true},
	"rainbow": {false, false, true, true},
	"strobe":  {true, false, true, true},
	"off":     {false, false, false, false},
}

// activeButton returns the key of the button with the .active CSS class,
// or the fallback value if none is found.
func activeButton(btns map[string]*gtk.Button, fallback string) string {
	for k, b := range btns {
		if b.HasCSSClass("active") {
			return k
		}
	}
	return fallback
}

// syncModeVis shows/hides color and speed sections based on the active mode.
// Safe to call at any time (including during sync).
func (w *Window) syncModeVis() {
	mode := activeButton(w.modeButtons, "static")
	v, ok := modeVisMap[mode]
	if !ok {
		v = modeVis{true, true, true, true}
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
	if w.brightBox != nil {
		w.brightBox.SetVisible(v.brightness)
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
	w.syncCustomView()
	if w.headerTelemetry != nil {
		w.headerTelemetry.SetLabel(fmt.Sprintf("%d°C · %d RPM", w.state.Temperature, w.state.FanRPM))
	}
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
	mode := ls.Mode
	if !ls.Enabled {
		mode = "off"
	}
	setActiveButton(w.modeButtons, mode)
	if w.color1 != nil && ls.Color != "" {
		w.color1.hex = strings.ToUpper(ls.Color)
	}
	if w.color2 != nil && ls.Color2 != "" {
		w.color2.hex = strings.ToUpper(ls.Color2)
	}
	w.updateSwatches()
	setActiveButton(w.speedBtns, ls.Speed)
	if w.brightScale != nil {
		w.brightScale.SetValue(float64(ls.Brightness))
	}
	w.syncModeVis()
}

// syncProfile highlights the profile button matching the daemon state.
func (w *Window) syncProfile() {
	if w.state == nil || w.state.Profile == "" {
		return
	}
	setActiveButton(w.profileBtns, w.state.Profile)
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
	color1 := defaultColor1
	if w.color1 != nil {
		color1 = w.color1.hex
	}
	color2 := defaultColor2
	if w.color2 != nil {
		color2 = w.color2.hex
	}

	mode := activeButton(w.modeButtons, defaultMode)
	speed := activeButton(w.speedBtns, defaultSpeed)

	brightness := defaultBrightness
	if w.brightScale != nil {
		brightness = int(w.brightScale.Value())
	}

	// "off" uses the daemon's dedicated off command so that Enabled=false
	// is persisted and survives a reboot.
	if mode == "off" {
		slog.Debug("sendApply: calling daemon off", "device", w.tab)
		start := time.Now()
		if _, err := api.SendOff(w.tab); err != nil {
			slog.Warn("off failed", "err", err, "elapsed", time.Since(start))
		} else {
			slog.Debug("sendApply: off done", "elapsed", time.Since(start))
		}
		return
	}

	slog.Debug("sendApply: calling daemon", "device", w.tab, "mode", mode, "brightness", brightness)
	start := time.Now()
	if _, err := api.SendApply(w.tab, color1, color2, mode, speed, brightness); err != nil {
		slog.Warn("apply failed", "err", err, "elapsed", time.Since(start))
	} else {
		slog.Debug("sendApply: done", "elapsed", time.Since(start))
	}
}

// sendProfileSet sends a profile change to the daemon.
func (w *Window) sendProfileSet(prof string) {
	slog.Debug("sendProfileSet: calling daemon", "profile", prof)
	start := time.Now()
	if _, err := api.SendProfileSet(prof); err != nil {
		slog.Warn("profile set failed", "profile", prof, "err", err, "elapsed", time.Since(start))
	} else {
		slog.Debug("sendProfileSet: done", "elapsed", time.Since(start))
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
				val := int(sc.Value())
				slog.Debug("sendBatteryLimitSet: calling daemon", "limit", val)
				start := time.Now()
				if _, err := api.SendBatteryLimitSet(val); err != nil {
					slog.Warn("battery limit set failed", "err", err, "elapsed", time.Since(start))
				} else {
					slog.Debug("sendBatteryLimitSet: done", "elapsed", time.Since(start))
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
	slog.Debug("sendOverdriveSet: calling daemon", "value", value)
	start := time.Now()
	if _, err := api.SendPanelOverdriveSet(value); err != nil {
		slog.Warn("panel overdrive set failed", "value", value, "err", err, "elapsed", time.Since(start))
	} else {
		slog.Debug("sendOverdriveSet: done", "elapsed", time.Since(start))
	}
}

// sendBootSoundSet sends a boot sound change to the daemon.
func (w *Window) sendBootSoundSet(value int) {
	slog.Debug("sendBootSoundSet: calling daemon", "value", value)
	start := time.Now()
	if _, err := api.SendBootSoundSet(value); err != nil {
		slog.Warn("boot sound set failed", "value", value, "err", err, "elapsed", time.Since(start))
	} else {
		slog.Debug("sendBootSoundSet: done", "elapsed", time.Since(start))
	}
}
