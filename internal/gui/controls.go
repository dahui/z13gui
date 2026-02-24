package gui

// controls.go — builds the entire drawer widget tree, theme picker view,
// and HSL color picker view. All views live in a gtk.Stack (both KDE and
// gamescope modes) for consistent gamepad navigation.

import (
	"fmt"
	"strings"

	"github.com/dahui/z13gui/internal/theme"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// addTouchActivate works around a GTK4 X11 issue where CheckButton and Switch
// widgets don't receive touch events properly in gamescope/XWayland (their
// internal GestureClick uses BUBBLE phase, which fails for touch). Adding a
// CAPTURE-phase, touch-only gesture ensures touch taps activate these widgets.
// Mouse input is unaffected (SetTouchOnly).
func addTouchActivate(widget gtk.Widgetter, onTap func()) {
	gesture := gtk.NewGestureClick()
	gesture.SetTouchOnly(true)
	gesture.SetPropagationPhase(gtk.PhaseCapture)
	gesture.ConnectReleased(func(nPress int, x, y float64) {
		onTap()
	})
	gtk.BaseWidget(widget).AddController(gesture)
}

// buildContent builds the scrolled content box and returns it as the window child.
// Content, theme view, and color picker view are in a gtk.Stack so views can be
// swapped for gamepad navigation (and in gamescope where popovers don't work).
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

	// Stack with main, theme, and color views — used in both modes.
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

	outer.Append(w.buildBottomBar())

	w.buildMainFocusList()
	w.buildThemeFocusList()
	w.buildColorFocusList()
	w.focusItems = w.mainFocusItems

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

	w.paletteBtn = gtk.NewButton()
	w.paletteBtn.SetIconName("preferences-desktop-color-symbolic")
	w.paletteBtn.SetTooltipText("Choose theme")
	w.paletteBtn.ConnectClicked(func() { w.showThemeView() })
	bar.Append(w.paletteBtn)

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
	if w.gamescope {
		addTouchActivate(s, func() { s.SetActive(!s.Active()) })
	}
	*sw = s
	box.Append(lbl)
	box.Append(s)
	return box
}

// buildThemeView builds the theme picker as a full scrollable view.
func (w *Window) buildThemeView() *gtk.Box {
	view := gtk.NewBox(gtk.OrientationVertical, 0)

	w.themeBackBtn = gtk.NewButton()
	w.themeBackBtn.SetIconName("go-previous-symbolic")
	w.themeBackBtn.AddCSSClass("view-back-btn")
	w.themeBackBtn.ConnectClicked(func() { w.showMainView() })

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.SetMarginTop(10)
	header.SetMarginBottom(6)
	header.SetMarginStart(14)
	header.Append(w.themeBackBtn)
	lbl := gtk.NewLabel("Theme")
	lbl.SetHAlign(gtk.AlignStart)
	lbl.AddCSSClass("drawer-title")
	header.Append(lbl)
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
// Also collects widget references in w.themeRadios and w.themeDots for
// building the theme focus list.
func (w *Window) appendThemeChoices(box *gtk.Box) {
	w.themeRadios = nil
	w.themeDots = nil

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
		if w.gamescope {
			addTouchActivate(btn, func() { btn.SetActive(true) })
		}
		w.themeRadios = append(w.themeRadios, btn)
		box.Append(btn)

		// Accent dots — shown for themes that have accent variants.
		var dots []*gtk.Button
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
				dots = append(dots, dot)
				row.Append(dot)
			}
			box.Append(dotsGrid)
		}
		w.themeDots = append(w.themeDots, dots)
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
		if w.gamescope {
			addTouchActivate(customBtn, func() { customBtn.SetActive(true) })
		}
		w.themeRadios = append(w.themeRadios, customBtn)
		box.Append(customBtn)

		var dots []*gtk.Button
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
				dots = append(dots, dot)
				row.Append(dot)
			}
			box.Append(dotsGrid)
		}
		w.themeDots = append(w.themeDots, dots)
	}
}

// buildColorPickerView builds the HSL color picker view.
// Contains preset buttons, hue/saturation/lightness sliders, and a preview swatch.
func (w *Window) buildColorPickerView() *gtk.Box {
	view := gtk.NewBox(gtk.OrientationVertical, 8)
	view.SetMarginStart(12)
	view.SetMarginEnd(12)

	// Header: back button + dynamic title.
	w.colorViewTitle = gtk.NewLabel("COLOR")
	w.colorBackBtn = gtk.NewButton()
	w.colorBackBtn.SetIconName("go-previous-symbolic")
	w.colorBackBtn.AddCSSClass("view-back-btn")
	w.colorBackBtn.ConnectClicked(func() { w.showMainView() })

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.SetMarginTop(10)
	header.SetMarginBottom(6)
	header.Append(w.colorBackBtn)
	w.colorViewTitle.SetHAlign(gtk.AlignStart)
	w.colorViewTitle.AddCSSClass("drawer-title")
	header.Append(w.colorViewTitle)
	view.Append(header)

	// 8 preset buttons.
	w.colorPickerPresets = nil
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
		w.colorPickerPresets = append(w.colorPickerPresets, btn)
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

// buildHSLScale creates a Scale for an HSL component.
func (w *Window) buildHSLScale(name string, min, max float64) *gtk.Scale {
	sc := gtk.NewScaleWithRange(gtk.OrientationHorizontal, min, max, 1)
	sc.SetDigits(0)
	sc.SetDrawValue(true)
	sc.SetFocusable(false)
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

// showMainView switches the view stack to the main drawer view.
func (w *Window) showMainView() {
	if w.viewStack != nil {
		w.viewStack.SetVisibleChildName("main")
		w.swapFocusList(w.mainFocusItems)
	}
}

// showThemeView switches the view stack to the theme picker view.
func (w *Window) showThemeView() {
	if w.viewStack != nil {
		w.viewStack.SetVisibleChildName("theme")
		w.swapFocusList(w.themeFocusItems)
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

	if w.gamescope {
		addTouchActivate(kb, func() { kb.SetActive(true) })
		addTouchActivate(lb, func() { lb.SetActive(true) })
	}

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
		if w.gamescope {
			addTouchActivate(btn, func() { btn.SetActive(true) })
		}
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
		if w.gamescope {
			addTouchActivate(btn, func() { btn.SetActive(true) })
		}
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
	sc.SetFocusable(false)
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
		if w.gamescope {
			addTouchActivate(btn, func() { btn.SetActive(true) })
		}
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
	sc.SetFocusable(false)
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

// buildMainFocusList builds the 2D focus grid for the main drawer view.
// Items are arranged by visual row/col matching the drawer layout.
func (w *Window) buildMainFocusList() {
	var items []focusItem
	boxVisible := func(box *gtk.Box) func() bool {
		return func() bool { return box.IsVisible() }
	}

	// Profiles — stacked vertically, one per row.
	for i, p := range profiles {
		btn := w.profileBtns[p]
		items = append(items, focusItem{
			widget: btn, row: i, col: 0,
			section:    "profile",
			onActivate: func() { btn.SetActive(true) },
		})
	}

	// Battery slider.
	battLeft, battRight, battGet, battSet := scaleAdjust(w.battScale, 5)
	items = append(items, focusItem{
		widget: w.battScale, row: 3, col: 0,
		section:  "battery",
		editable: true,
		onLeft:   battLeft, onRight: battRight,
		getValue: battGet, setValue: battSet,
	})

	// Device tabs — horizontal row.
	for col, btn := range []*gtk.CheckButton{w.tabKB, w.tabLB} {
		btn := btn
		items = append(items, focusItem{
			widget: btn, row: 4, col: col,
			section:    "tabs",
			onActivate: func() { btn.SetActive(true) },
		})
	}

	// Mode buttons — 3x2 grid.
	for i, m := range modeOrder {
		btn := w.modeButtons[m]
		items = append(items, focusItem{
			widget: btn, row: 5 + i/3, col: i % 3,
			section:    "mode",
			onActivate: func() { btn.SetActive(true) },
		})
	}

	// Color 1 presets — horizontal row of 8 buttons.
	if w.color1 != nil {
		vis := boxVisible(w.color1Box)
		for col, btn := range w.color1.presetBtns {
			btn := btn
			items = append(items, focusItem{
				widget: btn, row: 7, col: col,
				section: "color1", isVisible: vis,
				onActivate: func() { btn.Activate() },
			})
		}
		// Custom button on its own row below presets.
		items = append(items, focusItem{
			widget: w.color1.customBtn, row: 8, col: 0,
			section: "color1", isVisible: vis,
			onActivate: func() { w.showColorView(w.color1) },
		})
	}

	// Color 2 presets.
	if w.color2 != nil {
		vis := boxVisible(w.color2Box)
		for col, btn := range w.color2.presetBtns {
			btn := btn
			items = append(items, focusItem{
				widget: btn, row: 9, col: col,
				section: "color2", isVisible: vis,
				onActivate: func() { btn.Activate() },
			})
		}
		items = append(items, focusItem{
			widget: w.color2.customBtn, row: 10, col: 0,
			section: "color2", isVisible: vis,
			onActivate: func() { w.showColorView(w.color2) },
		})
	}

	// Speed buttons — horizontal row.
	speedOrder := []string{"slow", "normal", "fast"}
	for col, s := range speedOrder {
		btn := w.speedBtns[s]
		items = append(items, focusItem{
			widget: btn, row: 11, col: col,
			section: "speed", isVisible: boxVisible(w.speedBox),
			onActivate: func() { btn.SetActive(true) },
		})
	}

	// Brightness slider.
	brLeft, brRight, brGet, brSet := scaleAdjust(w.brightScale, 1)
	items = append(items, focusItem{
		widget: w.brightScale, row: 12, col: 0,
		section: "brightness", isVisible: boxVisible(w.brightBox),
		editable: true,
		onLeft:   brLeft, onRight: brRight,
		getValue: brGet, setValue: brSet,
	})

	// Footer: theme button, overdrive, boot sound.
	col := 0
	items = append(items, focusItem{
		widget: w.paletteBtn, row: 13, col: col,
		section:    "footer",
		onActivate: func() { w.showThemeView() },
	})
	col++
	if w.overdriveSwitch != nil {
		sw := w.overdriveSwitch
		items = append(items, focusItem{
			widget: sw, row: 13, col: col,
			section:    "footer",
			onActivate: func() { sw.SetActive(!sw.Active()) },
		})
		col++
	}
	if w.bootSoundSwitch != nil {
		sw := w.bootSoundSwitch
		items = append(items, focusItem{
			widget: sw, row: 13, col: col,
			section:    "footer",
			onActivate: func() { sw.SetActive(!sw.Active()) },
		})
	}

	w.mainFocusItems = items
}

// buildThemeFocusList builds the 2D focus grid for the theme picker view.
// Must be called after appendThemeChoices has populated w.themeRadios/w.themeDots.
func (w *Window) buildThemeFocusList() {
	var items []focusItem
	const dotsPerRow = 7

	// Row 0: back button.
	if w.themeBackBtn != nil {
		items = append(items, focusItem{
			widget: w.themeBackBtn, row: 0, col: 0,
			section:    "nav",
			onActivate: func() { w.showMainView() },
		})
	}

	row := 1
	for i, btn := range w.themeRadios {
		btn := btn
		items = append(items, focusItem{
			widget: btn, row: row, col: 0,
			section:    "theme",
			onActivate: func() { btn.SetActive(true) },
		})
		row++

		// Accent dots for this theme.
		if i < len(w.themeDots) && len(w.themeDots[i]) > 0 {
			for j, dot := range w.themeDots[i] {
				dot := dot
				items = append(items, focusItem{
					widget: dot, row: row + j/dotsPerRow, col: j % dotsPerRow,
					section:    "theme",
					onActivate: func() { dot.Activate() },
				})
			}
			row += (len(w.themeDots[i])-1)/dotsPerRow + 1
		}
	}

	w.themeFocusItems = items
}

// buildColorFocusList builds the 2D focus grid for the HSL color picker view.
func (w *Window) buildColorFocusList() {
	var items []focusItem

	// Row 0: back button.
	if w.colorBackBtn != nil {
		items = append(items, focusItem{
			widget: w.colorBackBtn, row: 0, col: 0,
			section:    "nav",
			onActivate: func() { w.showMainView() },
		})
	}

	// Row 1: color presets.
	for col, btn := range w.colorPickerPresets {
		btn := btn
		items = append(items, focusItem{
			widget: btn, row: 1, col: col,
			section:    "presets",
			onActivate: func() { btn.Activate() },
		})
	}

	// Rows 2-4: HSL sliders (editable).
	for i, sc := range []*gtk.Scale{w.colorHue, w.colorSat, w.colorLit} {
		oL, oR, gV, sV := scaleAdjust(sc, 5)
		items = append(items, focusItem{
			widget: sc, row: 2 + i, col: 0,
			section:  "sliders",
			editable: true,
			onLeft:   oL, onRight: oR,
			getValue: gV, setValue: sV,
		})
	}

	w.colorFocusItems = items
}
