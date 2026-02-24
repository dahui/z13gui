// Package gamepad reads Linux evdev gamepad events and dispatches normalized
// actions to the GUI. It scans /dev/input/event* for gamepad devices, reads
// events in background goroutines, and translates them into Action values.
// Only dispatches when the overlay is visible so games keep their input.
//
// Permissions: on modern systemd (Arch, Fedora, Ubuntu 22.04+), the uaccess
// udev tag in 70-uaccess.rules grants the active session user ACL access to
// joystick devices automatically — no input group membership needed. If device
// access fails, the device is silently skipped (graceful degradation).
package gamepad

import (
	"log/slog"
	"os"
	"sync"
	"time"

	evdev "github.com/holoplot/go-evdev"
)

// Action represents a normalized gamepad input dispatched to the GUI.
type Action int

const (
	ActionUp     Action = iota // D-pad up
	ActionDown                 // D-pad down
	ActionLeft                 // D-pad left
	ActionRight                // D-pad right
	ActionAccept               // A / BTN_SOUTH — activate focused widget
	ActionBack                 // B / BTN_EAST — dismiss / go back
	ActionBumpL                // Left shoulder (LB / BTN_TL)
	ActionBumpR                // Right shoulder (RB / BTN_TR)
)

// Handler is called on the GTK main thread for each gamepad action.
type Handler func(Action)

// Reader manages gamepad device discovery and event reading.
type Reader struct {
	handler   Handler
	isVisible func() bool
	dispatch  func(func()) // wraps glib.IdleAdd; injected to avoid glib import

	mu      sync.Mutex
	devices map[string]*evdev.InputDevice // tracked device paths → open device
	grabbed bool                          // true while overlay is visible (exclusive grab)
	stop    chan struct{}
}

// New creates a Reader. handler is called (via dispatch) for each action.
// isVisible gates dispatch so events are ignored when the overlay is hidden.
// dispatch must schedule f on the GTK main thread (typically glib.IdleAdd wrapper).
func New(handler Handler, isVisible func() bool, dispatch func(func())) *Reader {
	return &Reader{
		handler:   handler,
		isVisible: isVisible,
		dispatch:  dispatch,
		devices:   make(map[string]*evdev.InputDevice),
		stop:      make(chan struct{}),
	}
}

// Run scans for gamepad devices and reads events. Blocks until Stop is called.
func (r *Reader) Run() {
	r.scan()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.scan()
		}
	}
}

// Stop terminates the reader and all device goroutines.
func (r *Reader) Stop() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

// GrabAll acquires exclusive access (EVIOCGRAB) on all tracked gamepad devices
// so events are not delivered to other readers (e.g. the background game).
// New devices discovered while grabbed are auto-grabbed in tryOpen.
func (r *Reader) GrabAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grabbed = true
	for path, dev := range r.devices {
		if err := dev.Grab(); err != nil {
			slog.Debug("gamepad: grab failed", "path", path, "err", err)
		}
	}
}

// UngrabAll releases exclusive access on all tracked gamepad devices,
// allowing the background game to receive events again.
func (r *Reader) UngrabAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grabbed = false
	for _, dev := range r.devices {
		_ = dev.Ungrab()
	}
}

// scan enumerates /dev/input/event* and starts readers for new gamepad devices.
func (r *Reader) scan() {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		slog.Debug("gamepad: scan failed", "err", err)
		return
	}
	for _, p := range paths {
		r.mu.Lock()
		_, tracked := r.devices[p.Path]
		r.mu.Unlock()
		if tracked {
			continue
		}
		if r.tryOpen(p.Path) {
			slog.Info("gamepad: device found", "path", p.Path, "name", p.Name)
		}
	}
}

// tryOpen opens a device and checks for gamepad capabilities. If it's a
// gamepad, starts a readLoop goroutine and returns true.
func (r *Reader) tryOpen(path string) bool {
	dev, err := evdev.OpenWithFlags(path, os.O_RDONLY)
	if err != nil {
		return false
	}
	if !isGamepad(dev) {
		_ = dev.Close()
		return false
	}
	r.mu.Lock()
	r.devices[path] = dev
	if r.grabbed {
		if err := dev.Grab(); err != nil {
			slog.Debug("gamepad: grab failed on open", "path", path, "err", err)
		}
	}
	r.mu.Unlock()
	go r.readLoop(path, dev)
	return true
}

// isGamepad checks whether a device has gamepad button capabilities.
func isGamepad(dev *evdev.InputDevice) bool {
	keys := dev.CapableEvents(evdev.EV_KEY)
	for _, k := range keys {
		if k == evdev.BTN_SOUTH || k == evdev.BTN_GAMEPAD {
			return true
		}
	}
	return false
}

// repeat timing constants.
const (
	repeatInitial  = 400 * time.Millisecond
	repeatInterval = 120 * time.Millisecond
)

// readLoop reads events from a single device. Runs until the device
// disconnects or the reader is stopped.
func (r *Reader) readLoop(path string, dev *evdev.InputDevice) {
	defer func() {
		r.mu.Lock()
		if r.grabbed {
			_ = dev.Ungrab()
		}
		delete(r.devices, path)
		r.mu.Unlock()
		_ = dev.Close()
		slog.Info("gamepad: device disconnected", "path", path)
	}()

	var repeatMu sync.Mutex
	var repeatTimer *time.Timer

	stopRepeat := func() {
		repeatMu.Lock()
		if repeatTimer != nil {
			repeatTimer.Stop()
			repeatTimer = nil
		}
		repeatMu.Unlock()
	}
	defer stopRepeat()

	startRepeat := func(a Action) {
		repeatMu.Lock()
		if repeatTimer != nil {
			repeatTimer.Stop()
		}
		var tick func()
		tick = func() {
			r.emit(a)
			repeatMu.Lock()
			if repeatTimer != nil {
				repeatTimer = time.AfterFunc(repeatInterval, tick)
			}
			repeatMu.Unlock()
		}
		repeatTimer = time.AfterFunc(repeatInitial, tick)
		repeatMu.Unlock()
	}

	for {
		select {
		case <-r.stop:
			return
		default:
		}

		ev, err := dev.ReadOne()
		if err != nil {
			return // device disconnected
		}

		switch ev.Type {
		case evdev.EV_KEY:
			switch ev.Value {
			case 1: // key down
				if a, ok := buttonToAction(ev.Code); ok {
					r.emit(a)
					if isDirectional(a) {
						startRepeat(a)
					}
				}
			case 0: // key up
				if a, ok := buttonToAction(ev.Code); ok {
					if isDirectional(a) {
						stopRepeat()
					}
				}
			}

		case evdev.EV_ABS:
			switch ev.Code {
			case evdev.ABS_HAT0Y:
				if ev.Value < 0 {
					r.emit(ActionUp)
					startRepeat(ActionUp)
				} else if ev.Value > 0 {
					r.emit(ActionDown)
					startRepeat(ActionDown)
				} else {
					stopRepeat()
				}
			case evdev.ABS_HAT0X:
				if ev.Value < 0 {
					r.emit(ActionLeft)
					startRepeat(ActionLeft)
				} else if ev.Value > 0 {
					r.emit(ActionRight)
					startRepeat(ActionRight)
				} else {
					stopRepeat()
				}
			}
		}
	}
}

// emit dispatches an action to the GUI thread if the overlay is visible.
func (r *Reader) emit(a Action) {
	if !r.isVisible() {
		return
	}
	r.dispatch(func() { r.handler(a) })
}

// buttonToAction maps evdev button codes to actions.
func buttonToAction(code evdev.EvCode) (Action, bool) {
	switch code {
	case evdev.BTN_SOUTH:
		return ActionAccept, true
	case evdev.BTN_EAST:
		return ActionBack, true
	case evdev.BTN_TL:
		return ActionBumpL, true
	case evdev.BTN_TR:
		return ActionBumpR, true
	case evdev.BTN_DPAD_UP:
		return ActionUp, true
	case evdev.BTN_DPAD_DOWN:
		return ActionDown, true
	case evdev.BTN_DPAD_LEFT:
		return ActionLeft, true
	case evdev.BTN_DPAD_RIGHT:
		return ActionRight, true
	default:
		return 0, false
	}
}

// isDirectional returns true for actions that should auto-repeat when held.
func isDirectional(a Action) bool {
	return a == ActionUp || a == ActionDown || a == ActionLeft || a == ActionRight
}
