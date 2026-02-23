package gui

// controls.go — builds the entire drawer widget tree, theme picker, and
// gamescope-specific view alternatives (theme view, HSL color picker view).

import (
	"fmt"
	"strings"

	"github.com/dahui/z13gui/internal/theme"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// buildContent builds the scrolled content box and returns it as the window child.
// In gamescope mode the scrolled content, theme view, and color picker view are
// wrapped in a gtk.Stack so views can be swapped without using popovers (which
// create separate X11 windows that gamescope doesn't composite).
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
	w.color1 = w.newColorInput("FF0000", "color1-swatch", "COLOR 1")
	w.color2 = w.newColorInput("000000", "color2-swatch", "COLOR 2")
	w.updateSwatches()

	w.color1Box = colorSubBox("COLOR 1", w.color1.row)
	w.color2Box = colorSubBox("COLOR 2", w.color2.row)
	inner.Append(w.color1Box)
	inner.Append(w.color2Box)

	w.speedBox = w.buildSpeedBox()
	inner.Append(w.speedBox)
	w.brightBox = w.buildBrightnessBox()
	inner.Append(w.brightBox)

	// Set initial visibility based on default mode (static).
	w.syncModeVis()

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetVExpand(true)
	scroll.SetChild(inner)

	if w.gamescope {
		w.viewStack = gtk.NewStack()
		w.viewStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
		w.viewStack.SetVExpand(true)

		mainPage := gtk.NewBox(gtk.OrientationVertical, 0)
		mainPage.Append(title)
		mainPage.Append(scroll)
		w.viewStack.AddNamed(mainPage, "main")
		w.viewStack.AddNamed(w.buildThemeView(), "theme")
		w.viewStack.AddNamed(w.buildColorPickerView(), "color")
		w.viewStack.SetVisibleChildName("main")
		outer.Append(w.viewStack)
	} else {
		outer.Append(title)
		outer.Append(scroll)
	}

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

	if w.gamescope {
		paletteBtn := gtk.NewButton()
		paletteBtn.SetIconName("preferences-desktop-color-symbolic")
		paletteBtn.SetTooltipText("Choose theme")
		paletteBtn.ConnectClicked(func() { w.showThemeView() })
		bar.Append(paletteBtn)
	} else {
		paletteBtn := gtk.NewMenuButton()
		paletteBtn.SetIconName("preferences-desktop-color-symbolic")
		paletteBtn.SetTooltipText("Choose theme")
		paletteBtn.SetPopover(w.buildThemePopover())
		bar.Append(paletteBtn)
	}

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
// Used in KDE mode; in gamescope mode buildThemeView is used instead.
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

	w.appendThemeChoices(box)

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetMaxContentHeight(500)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetChild(box)
	pop.SetChild(scroll)
	return pop
}

// buildThemeView builds the theme picker as a full scrollable view for gamescope.
// Used instead of the popover which creates a separate X11 window.
func (w *Window) buildThemeView() *gtk.Box {
	view := gtk.NewBox(gtk.OrientationVertical, 0)

	header := w.buildViewHeader("Theme", func() { w.showMainView() })
	view.Append(header)

	content := gtk.NewBox(gtk.OrientationVertical, 2)
	content.SetMarginTop(4)
	content.SetMarginBottom(12)
	content.SetMarginStart(12)
	content.SetMarginEnd(12)
	w.appendThemeChoices(content)

	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetVExpand(true)
	scroll.SetChild(content)
	view.Append(scroll)
	return view
}

// appendThemeChoices appends the theme radio buttons and accent dots to box.
// Shared between buildThemePopover (KDE) and buildThemeView (gamescope).
func (w *Window) appendThemeChoices(box *gtk.Box) {
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

			dotsGrid := gtk.NewBox(gtk.OrientationVertical, 4)
			dotsGrid.SetMarginStart(12)
			dotsGrid.SetMarginBottom(4)

			const dotsPerRow = 7
			var row *gtk.Box
			for i, ac := range t.Accents {
				ac := ac
				if i%dotsPerRow == 0 {
					row = gtk.NewBox(gtk.OrientationHorizontal, 4)
					dotsGrid.Append(row)
				}
				dot := gtk.NewButton()
				dot.AddCSSClass("color-preset")
				dot.SetHExpand(true)
				if id == activeCfg.Theme && ac.ID == activeCfg.Accent {
					dot.AddCSSClass("accent-dot-active")
				}
				provider := gtk.NewCSSProvider()
				provider.LoadFromString("button.color-preset { background: " + ac.Hex + "; }")
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

			dotsGrid := gtk.NewBox(gtk.OrientationVertical, 4)
			dotsGrid.SetMarginStart(12)
			dotsGrid.SetMarginBottom(4)

			const dotsPerRow = 7
			var row *gtk.Box
			for i, ac := range w.customAccents {
				ac := ac
				if i%dotsPerRow == 0 {
					row = gtk.NewBox(gtk.OrientationHorizontal, 4)
					dotsGrid.Append(row)
				}
				dot := gtk.NewButton()
				dot.AddCSSClass("color-preset")
				dot.SetHExpand(true)
				if ac.ID == activeCfg.Accent {
					dot.AddCSSClass("accent-dot-active")
				}
				provider := gtk.NewCSSProvider()
				provider.LoadFromString("button.color-preset { background: " + ac.Hex + "; }")
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
}

// buildColorPickerView builds the HSL color picker view for gamescope mode.
// Contains preset buttons, hue/saturation/lightness sliders, and a preview swatch.
func (w *Window) buildColorPickerView() *gtk.Box {
	view := gtk.NewBox(gtk.OrientationVertical, 8)
	view.SetMarginStart(12)
	view.SetMarginEnd(12)

	// Header: back button + dynamic title.
	w.colorViewTitle = gtk.NewLabel("COLOR")
	view.Append(w.buildColorViewHeader())

	// 8 preset buttons.
	presetsRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	for _, hex := range presetColors {
		h := hex
		btn := gtk.NewButton()
		btn.AddCSSClass("color-preset")
		btn.SetHExpand(true)
		p := gtk.NewCSSProvider()
		p.LoadFromString(fmt.Sprintf("button.color-preset { background: #%s; }", h))
		btn.StyleContext().AddProvider(p, gtk.STYLE_PROVIDER_PRIORITY_USER+5) //nolint:staticcheck // per-widget dynamic color
		btn.ConnectClicked(func() { w.colorPickerPresetClicked(h) })
		presetsRow.Append(btn)
	}
	view.Append(presetsRow)

	// HSL sliders.
	w.colorHue = w.buildHSLScale("HUE", 0, 360)
	w.colorSat = w.buildHSLScale("SATURATION", 0, 100)
	w.colorLit = w.buildHSLScale("LIGHTNESS", 0, 100)

	view.Append(hslScaleBox("HUE", w.colorHue))
	view.Append(hslScaleBox("SATURATION", w.colorSat))
	view.Append(hslScaleBox("LIGHTNESS", w.colorLit))

	// Preview swatch + hex label.
	w.colorSwatchProv = gtk.NewCSSProvider()
	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(), w.colorSwatchProv,
		gtk.STYLE_PROVIDER_PRIORITY_USER+10,
	)

	w.colorPreview = gtk.NewBox(gtk.OrientationHorizontal, 0)
	w.colorPreview.AddCSSClass("color-swatch")
	w.colorPreview.SetName("color-picker-preview")

	w.colorHexLabel = gtk.NewLabel("#FF0000")
	w.colorHexLabel.AddCSSClass("section-label")

	previewRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	previewRow.SetMarginTop(4)
	previewRow.Append(w.colorPreview)
	previewRow.Append(w.colorHexLabel)
	view.Append(previewRow)

	return view
}

// buildColorViewHeader builds the header for the color picker view with a back
// button and the dynamic color title label.
func (w *Window) buildColorViewHeader() *gtk.Box {
	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.SetMarginTop(10)
	header.SetMarginBottom(6)
	backBtn := gtk.NewButton()
	backBtn.SetIconName("go-previous-symbolic")
	backBtn.AddCSSClass("view-back-btn")
	backBtn.ConnectClicked(func() { w.showMainView() })
	header.Append(backBtn)
	w.colorViewTitle.SetHAlign(gtk.AlignStart)
	w.colorViewTitle.AddCSSClass("drawer-title")
	header.Append(w.colorViewTitle)
	return header
}

// buildHSLScale creates a Scale for an HSL component.
func (w *Window) buildHSLScale(name string, min, max float64) *gtk.Scale {
	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, min, max, 1)
	sc.SetDigits(0)
	sc.SetDrawValue(true)
	sc.ConnectValueChanged(func() { w.onHSLChanged() })
	return sc
}

// hslScaleBox wraps a section label + scale into a box.
func hslScaleBox(label string, sc *gtk.Scale) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.Append(sectionLabel(label))
	box.Append(sc)
	return box
}

// buildViewHeader builds a header bar with a back button and title label.
func (w *Window) buildViewHeader(titleText string, onBack func()) *gtk.Box {
	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.SetMarginTop(10)
	header.SetMarginBottom(6)
	header.SetMarginStart(14)
	backBtn := gtk.NewButton()
	backBtn.SetIconName("go-previous-symbolic")
	backBtn.AddCSSClass("view-back-btn")
	backBtn.ConnectClicked(func() { onBack() })
	header.Append(backBtn)
	lbl := gtk.NewLabel(titleText)
	lbl.SetHAlign(gtk.AlignStart)
	lbl.AddCSSClass("drawer-title")
	header.Append(lbl)
	return header
}

// showMainView switches the gamescope view stack to the main drawer view.
func (w *Window) showMainView() {
	if w.viewStack != nil {
		w.viewStack.SetVisibleChildName("main")
	}
}

// showThemeView switches the gamescope view stack to the theme picker view.
func (w *Window) showThemeView() {
	if w.viewStack != nil {
		w.viewStack.SetVisibleChildName("theme")
	}
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

// buildProfileSection creates the profile radio buttons (quiet/balanced/performance).
func (w *Window) buildProfileSection() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(sectionLabel("PROFILE"))

	row := gtk.NewBox(gtk.OrientationVertical, 4)
	var first *gtk.CheckButton
	for _, p := range profiles {
		prof := p
		btn := gtk.NewCheckButtonWithLabel(strings.Title(prof)) //nolint:staticcheck // strings.Title is fine for ASCII-only mode/speed/profile labels
		if first == nil {
			first = btn
		} else {
			btn.SetGroup(first)
		}
		btn.ConnectToggled(func() {
			if btn.Active() && !w.syncing {
				w.sendProfileSet(prof)
			}
		})
		w.profileBtns[prof] = btn
		row.Append(btn)
	}

	box.Append(row)
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
