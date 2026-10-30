// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

// Package tray exposes z13gui as a StatusNotifierItem with a profile menu.
package tray

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"deedles.dev/tray"
	"github.com/dahui/z13ctl/api"
	"github.com/dahui/z13gui/internal/profileinfo"
)

const (
	trayID     = "com.github.dahui.z13gui"
	trayTitle  = "z13gui"
	pollPeriod = 5 * time.Second
)

// Options wires tray actions into the running application. Callbacks run on the
// tray's D-Bus goroutine, so the caller is responsible for moving work onto the
// GTK main thread where required.
type Options struct {
	Toggle func()
	Quit   func()
}

// Tray is the StatusNotifierItem and its profile menu.
type Tray struct {
	item *tray.Item
	opts Options

	mu      sync.Mutex
	profile string
	icon    string
	items   map[string]*tray.MenuItem
	stop    chan struct{}
}

// New creates the tray icon and menu. It fails if no StatusNotifierWatcher is
// available on the session bus.
func New(opts Options) (*Tray, error) {
	item, err := tray.New(
		tray.ItemID(trayID),
		tray.ItemTitle(trayTitle),
		tray.ItemIconPixmap(profileIcon("")),
		tray.ItemToolTip("", nil, trayTitle, "Power profile"),
		tray.ItemIsMenu(false),
		tray.ItemHandler(tray.ActivateHandler(func(_, _ int) error {
			opts.Toggle()
			return nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("create status notifier item: %w", err)
	}

	t := &Tray{
		item:  item,
		opts:  opts,
		items: make(map[string]*tray.MenuItem),
		stop:  make(chan struct{}),
	}
	if err := t.buildMenu(); err != nil {
		_ = item.Close()
		return nil, err
	}
	t.refresh()
	go t.poll()
	return t, nil
}

// Close removes the tray icon.
func (t *Tray) Close() error {
	close(t.stop)
	return t.item.Close()
}

func (t *Tray) buildMenu() error {
	menu := t.item.Menu()

	if _, err := menu.AddChild(
		tray.MenuItemLabel("Power profile"),
		tray.MenuItemEnabled(false),
	); err != nil {
		return fmt.Errorf("add menu header: %w", err)
	}

	for _, name := range profileNames() {
		name := name
		item, err := menu.AddChild(
			tray.MenuItemLabel(profileinfo.Title(name)),
			tray.MenuItemToggleType(tray.Radio),
			tray.MenuItemToggleState(tray.Off),
			tray.MenuItemHandler(tray.ClickedHandler(func(any, uint32) error {
				go t.setProfile(name)
				return nil
			})),
		)
		if err != nil {
			return fmt.Errorf("add profile %q: %w", name, err)
		}
		t.mu.Lock()
		t.items[name] = item
		t.mu.Unlock()
	}

	if _, err := menu.AddChild(tray.MenuItemType(tray.Separator)); err != nil {
		return fmt.Errorf("add separator: %w", err)
	}

	if _, err := menu.AddChild(
		tray.MenuItemLabel("Open z13gui"),
		tray.MenuItemHandler(tray.ClickedHandler(func(any, uint32) error {
			t.opts.Toggle()
			return nil
		})),
	); err != nil {
		return fmt.Errorf("add open item: %w", err)
	}

	if _, err := menu.AddChild(
		tray.MenuItemLabel("Quit"),
		tray.MenuItemHandler(tray.ClickedHandler(func(any, uint32) error {
			t.opts.Quit()
			return nil
		})),
	); err != nil {
		return fmt.Errorf("add quit item: %w", err)
	}

	return nil
}

func (t *Tray) setProfile(name string) {
	handled, err := api.SendProfileSet(name)
	if err != nil {
		slog.Warn("tray: set profile", "profile", name, "err", err)
		return
	}
	if !handled {
		slog.Warn("tray: z13ctl daemon not running")
		return
	}
	t.refresh()
}

func (t *Tray) refresh() {
	current := t.current()
	handled, state, err := api.SendGetState()
	if err != nil {
		slog.Warn("tray: read state", "err", err)
	}
	if handled && state != nil && state.Profile != "" {
		current = state.Profile
	}

	t.mu.Lock()
	t.profile = current
	iconChanged := current != t.icon
	t.icon = current
	items := make(map[string]*tray.MenuItem, len(t.items))
	for name, item := range t.items {
		items[name] = item
	}
	t.mu.Unlock()

	if iconChanged {
		if err := t.item.SetProps(tray.ItemIconPixmap(profileIcon(current))); err != nil {
			slog.Debug("tray: update icon", "err", err)
		}
	}

	for name, item := range items {
		state := tray.Off
		if name == current {
			state = tray.On
		}
		if err := item.SetProps(tray.MenuItemToggleState(state)); err != nil {
			slog.Debug("tray: update profile item", "profile", name, "err", err)
		}
	}

	if err := t.item.SetProps(tray.ItemToolTip("", nil, "Profile: "+profileinfo.Title(current), profileinfo.Describe(current, state))); err != nil {
		slog.Debug("tray: update tooltip", "err", err)
	}
}

func (t *Tray) current() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.profile
}

func (t *Tray) poll() {
	ticker := time.NewTicker(pollPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			t.refresh()
		}
	}
}

// profileNames returns the firmware profiles followed by any custom profiles.
// It falls back to the firmware profiles when the daemon is not available.
func profileNames() []string {
	names := []string{"quiet", "balanced", "performance"}
	handled, raw, err := api.SendProfileList()
	if err != nil {
		slog.Warn("tray: list custom profiles", "err", err)
		return names
	}
	if !handled || raw == "" {
		return names
	}

	var custom []api.CustomProfile
	if err := json.Unmarshal([]byte(raw), &custom); err != nil {
		slog.Warn("tray: decode custom profiles", "err", err)
		return names
	}

	seen := make(map[string]bool, len(names)+len(custom))
	for _, name := range names {
		seen[name] = true
	}
	for _, p := range custom {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		names = append(names, p.Name)
	}
	return names
}
