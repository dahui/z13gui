package gui

// controls.go — builds the entire drawer widget tree and theme picker popover.

import (
	"strings"

	"github.com/dahui/z13gui/internal/theme"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// buildContent builds the scrolled content box and returns it as the window child.
func (w *Window) buildContent() gtk.Widgetter {
	outer := gtk.NewBox(gtk.OrientationVertical, 0)
	outer.AddCSSClass("drawer")

	// Fixed title — sits above the scroll area, always visible.
	title := gtk.NewLabel("z13ctl")
	title.SetHAlign(gtk.AlignStart)
	title.AddCSSClass("drawer-title")
	title.SetMarginTop(10)
	title.SetMarginBottom(6)
	title.SetMarginStart(14)
	outer.Append(title)

	inner := gtk.NewBox(gtk.OrientationVertical, 8)
	inner.SetMarginTop(4)
	inner.SetMarginBottom(12)
	inner.SetMarginStart(12)
	inner.SetMarginEnd(12)

	// TDP AND POWER section.
	inner.Append(groupLabel("TDP AND POWER"))
	inner.Append(w.buildProfileSection())
	inner.Append(w.buildBatterySection())
	inner.Append(separator())

	// RGB section.
	inner.Append(groupLabel("RGB"))
	inner.Append(w.buildTabRow())
	inner.Append(w.buildModeSection())

	// Initialize color inputs here so syncModeVis can reference them.
	w.color1 = w.newColorInput("FF0000", "color1-swatch")
	w.color2 = w.newColorInput("000000", "color2-swatch")
	w.updateSwatches()

	w.color1Box = colorSubBox("COLOR 1", w.color1.row)
	w.color2Box = colorSubBox("COLOR 2", w.color2.row)
	inner.Append(w.color1Box)
	inner.Append(w.color2Box)

	w.speedBox = w.buildSpeedBox()
	inner.Append(w.speedBox)
	inner.Append(w.buildBrightnessBox())

	// Set initial visibility based on default mode (static).
	w.syncModeVis()

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetVExpand(true)
	scroll.SetChild(inner)

	outer.Append(scroll)
	outer.Append(w.buildBottomBar())
	return outer
}

// buildBottomBar returns the fixed bottom bar containing the theme picker button
// and system toggles (panel overdrive, boot sound). It sits below the scroll
// area and is always visible (not scrolled).
func (w *Window) buildBottomBar() *gtk.Box {
	bar := gtk.NewBox(gtk.OrientationHorizontal, 4)
	bar.AddCSSClass("bottom-bar")
	bar.SetMarginTop(4)
	bar.SetMarginBottom(8)
	bar.SetMarginStart(10)

	paletteBtn := gtk.NewMenuButton()
	paletteBtn.SetIconName("preferences-desktop-color-symbolic")
	paletteBtn.SetTooltipText("Choose theme")
	paletteBtn.SetPopover(w.buildThemePopover())
	bar.Append(paletteBtn)

	// Spacer pushes toggles to the right.
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	bar.Append(spacer)

	bar.Append(w.buildToggle("Panel Overdrive", "Enable panel overdrive for faster pixel response (may cause ghosting)", &w.overdriveSwitch, func(active bool) {
		v := 0
		if active {
			v = 1
		}
		w.sendOverdriveSet(v)
	}))
	bar.Append(w.buildToggle("Boot Sound", "Play startup sound when the laptop powers on", &w.bootSoundSwitch, func(active bool) {
		v := 0
		if active {
			v = 1
		}
		w.sendBootSoundSet(v)
	}))

	return bar
}

// buildToggle creates a compact label + switch pair for the bottom bar.
func (w *Window) buildToggle(label, tooltip string, sw **gtk.Switch, onChange func(bool)) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationHorizontal, 4)
	box.SetTooltipText(tooltip)
	lbl := gtk.NewLabel(label)
	lbl.AddCSSClass("toggle-label")
	s := gtk.NewSwitch()
	s.ConnectStateSet(func(state bool) bool {
		if !w.syncing {
			onChange(state)
		}
		return false
	})
	*sw = s
	box.Append(lbl)
	box.Append(s)
	return box
}

// buildThemePopover builds the theme selection popover with accent color dots.
func (w *Window) buildThemePopover() *gtk.Popover {
	pop := gtk.NewPopover()
	pop.AddCSSClass("z13-popover")
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.SetMarginTop(6)
	box.SetMarginBottom(6)
	box.SetMarginStart(8)
	box.SetMarginEnd(8)

	// Popover title.
	title := gtk.NewLabel("Theme")
	title.SetXAlign(0)
	title.AddCSSClass("section-label")
	title.SetMarginBottom(4)
	box.Append(title)

	activeCfg := theme.LoadAppConfig()
	var first *gtk.CheckButton
	for _, t := range theme.Builtins {
		id := t.ID
		btn := gtk.NewCheckButtonWithLabel(t.Name)
		if first == nil {
			first = btn
		} else {
			btn.SetGroup(first)
		}
		if id == activeCfg.Theme {
			btn.SetActive(true)
		}
		btn.ConnectToggled(func() {
			if btn.Active() {
				w.applyTheme(id, "")
			}
		})
		box.Append(btn)

		// Accent dots — shown for themes that have accent variants.
		if len(t.Accents) > 0 {
			accentLabel := gtk.NewLabel("Accent Color")
			accentLabel.SetXAlign(0)
			accentLabel.AddCSSClass("accent-label")
			accentLabel.SetMarginStart(12)
			accentLabel.SetMarginTop(2)
			box.Append(accentLabel)

			dotsGrid := gtk.NewBox(gtk.OrientationVertical, 3)
			dotsGrid.SetMarginStart(12)
			dotsGrid.SetMarginBottom(4)

			const dotsPerRow = 7
			var row *gtk.Box
			for i, ac := range t.Accents {
				ac := ac
				if i%dotsPerRow == 0 {
					row = gtk.NewBox(gtk.OrientationHorizontal, 3)
					dotsGrid.Append(row)
				}
				dot := gtk.NewButton()
				dot.AddCSSClass("accent-dot")
				if id == activeCfg.Theme && ac.ID == activeCfg.Accent {
					dot.AddCSSClass("accent-dot-active")
				}
				provider := gtk.NewCSSProvider()
				provider.LoadFromString("button.accent-dot { background: " + ac.Hex + "; }")
				dot.StyleContext().AddProvider(provider, gtk.STYLE_PROVIDER_PRIORITY_USER+20) //nolint:staticcheck // per-widget dynamic color; no style-class alternative for unique hex backgrounds
				dot.SetTooltipText(ac.Name)
				dot.ConnectClicked(func() {
					btn.SetActive(true)
					w.applyTheme(id, ac.ID)
				})
				row.Append(dot)
			}
			box.Append(dotsGrid)
		}
	}

	// Custom theme entry — shown when theme.toml is active.
	if w.isCustomTheme {
		customBtn := gtk.NewCheckButtonWithLabel("Custom")
		if first != nil {
			customBtn.SetGroup(first)
		}
		customBtn.SetActive(true)
		// Clicking re-applies the base custom colors (resets accent to default).
		customBtn.ConnectToggled(func() {
			if customBtn.Active() {
				w.applyCustomAccent("")
			}
		})
		box.Append(customBtn)

		if len(w.customAccents) > 0 {
			accentLabel := gtk.NewLabel("Accent Color")
			accentLabel.SetXAlign(0)
			accentLabel.AddCSSClass("accent-label")
			accentLabel.SetMarginStart(12)
			accentLabel.SetMarginTop(2)
			box.Append(accentLabel)

			dotsGrid := gtk.NewBox(gtk.OrientationVertical, 3)
			dotsGrid.SetMarginStart(12)
			dotsGrid.SetMarginBottom(4)

			const dotsPerRow = 7
			var row *gtk.Box
			for i, ac := range w.customAccents {
				ac := ac
				if i%dotsPerRow == 0 {
					row = gtk.NewBox(gtk.OrientationHorizontal, 3)
					dotsGrid.Append(row)
				}
				dot := gtk.NewButton()
				dot.AddCSSClass("accent-dot")
				if ac.ID == activeCfg.Accent {
					dot.AddCSSClass("accent-dot-active")
				}
				provider := gtk.NewCSSProvider()
				provider.LoadFromString("button.accent-dot { background: " + ac.Hex + "; }")
				dot.StyleContext().AddProvider(provider, gtk.STYLE_PROVIDER_PRIORITY_USER+20) //nolint:staticcheck // per-widget dynamic color; no style-class alternative for unique hex backgrounds
				dot.SetTooltipText(ac.Name)
				dot.ConnectClicked(func() {
					w.applyCustomAccent(ac.ID)
				})
				row.Append(dot)
			}
			box.Append(dotsGrid)
		}
	}

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetMaxContentHeight(500)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetChild(box)
	pop.SetChild(scroll)
	return pop
}

// buildTabRow creates the Keyboard / Lightbar tab radio buttons.
func (w *Window) buildTabRow() *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 4)

	kb := gtk.NewCheckButtonWithLabel("Keyboard")
	kb.SetActive(true)
	lb := gtk.NewCheckButtonWithLabel("Lightbar")
	lb.SetGroup(kb)

	kb.AddCSSClass("tab-btn")
	lb.AddCSSClass("tab-btn")
	kb.SetHExpand(true)
	lb.SetHExpand(true)

	kb.ConnectToggled(func() {
		if kb.Active() {
			w.tab = "keyboard"
			w.syncLightingSection()
		}
	})
	lb.ConnectToggled(func() {
		if lb.Active() {
			w.tab = "lightbar"
			w.syncLightingSection()
		}
	})

	w.tabKB = kb
	w.tabLB = lb

	row.Append(kb)
	row.Append(lb)
	return row
}

// modeOrder defines the display order for lighting mode buttons.
var modeOrder = []string{
	"static", "breathe", "cycle",
	"rainbow", "strobe", "off",
}

// buildModeSection creates the 3x2 grid of lighting mode radio buttons.
func (w *Window) buildModeSection() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("MODE"))

	grid := gtk.NewGrid()
	grid.SetColumnSpacing(4)
	grid.SetRowSpacing(4)
	grid.AddCSSClass("mode-grid")
	grid.SetColumnHomogeneous(true)

	var first *gtk.CheckButton
	for i, m := range modeOrder {
		mode := m
		btn := gtk.NewCheckButtonWithLabel(strings.Title(mode)) //nolint:staticcheck // strings.Title is fine for ASCII-only mode/speed/profile labels
		if first == nil {
			first = btn
		} else {
			btn.SetGroup(first)
		}
		btn.ConnectToggled(func() {
			if btn.Active() {
				w.syncModeVis()
				w.sendApply()
			}
		})
		w.modeButtons[mode] = btn
		grid.Attach(btn, i%3, i/3, 1, 1)
	}

	box.Append(grid)
	return box
}

// buildSpeedBox creates the slow/normal/fast radio button row.
func (w *Window) buildSpeedBox() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("SPEED"))

	speedRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	speeds := []string{"slow", "normal", "fast"}
	var firstSpeed *gtk.CheckButton
	for _, s := range speeds {
		sp := s
		btn := gtk.NewCheckButtonWithLabel(strings.Title(sp)) //nolint:staticcheck // strings.Title is fine for ASCII-only mode/speed/profile labels
		if firstSpeed == nil {
			firstSpeed = btn
		} else {
			btn.SetGroup(firstSpeed)
		}
		btn.ConnectToggled(func() {
			if btn.Active() {
				w.sendApply()
			}
		})
		w.speedBtns[sp] = btn
		speedRow.Append(btn)
	}
	if normal, ok := w.speedBtns["normal"]; ok {
		normal.SetActive(true)
	}
	box.Append(speedRow)
	return box
}

// buildBrightnessBox creates the brightness scale (0–3).
func (w *Window) buildBrightnessBox() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("BRIGHTNESS"))

	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 3, 1)
	sc.SetDigits(0)
	sc.SetDrawValue(true)
	sc.SetValue(3)
	sc.ConnectValueChanged(func() {
		w.queueApply()
	})
	w.brightScale = sc
	box.Append(sc)
	return box
}

// profiles lists the available sysfs performance profiles.
var profiles = []string{"quiet", "balanced", "performance"}

// buildProfileSection creates the profile dropdown (quiet/balanced/performance).
func (w *Window) buildProfileSection() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("PROFILE"))

	labels := make([]string, len(profiles))
	for i, p := range profiles {
		labels[i] = strings.Title(p) //nolint:staticcheck // strings.Title is fine for ASCII-only mode/speed/profile labels
	}
	dd := gtk.NewDropDownFromStrings(labels)
	dd.NotifyProperty("selected", func() {
		if w.syncing {
			return
		}
		idx := dd.Selected()
		if int(idx) >= len(profiles) {
			return
		}
		w.sendProfileSet(profiles[idx])
	})
	w.profileDrop = dd
	box.Append(dd)
	return box
}

// buildBatterySection creates the battery charge limit scale (40–100%).
func (w *Window) buildBatterySection() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("BATTERY LIMIT"))

	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 40, 100, 1)
	sc.SetDigits(0)
	sc.SetDrawValue(true)
	sc.SetValue(80)
	w.battScale = sc
	w.initBatteryDebounce(sc)

	box.Append(sc)
	return box
}

// colorSubBox wraps a section label + content widget into a single Box,
// making it easy to show/hide the whole subsection at once.
func colorSubBox(label string, content gtk.Widgetter) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationVertical, 4)
	b.Append(sectionLabel(label))
	b.Append(content)
	return b
}

// sectionLabel creates a small-caps section label (e.g. "MODE", "SPEED").
func sectionLabel(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.SetHAlign(gtk.AlignStart)
	l.AddCSSClass("section-label")
	return l
}

// groupLabel creates a section group heading (e.g. "TDP AND POWER", "RGB").
func groupLabel(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.SetHAlign(gtk.AlignStart)
	l.AddCSSClass("section-group")
	return l
}

// separator creates a horizontal separator line.
func separator() *gtk.Separator {
	return gtk.NewSeparator(gtk.OrientationHorizontal)
}
