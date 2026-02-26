package gamepad

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// pidFilePath returns the path to the frozen-PID state file.
// Used for crash recovery: if z13gui is killed, the systemd ExecStopPost
// or signal handler reads this file to thaw the frozen Steam process.
func pidFilePath() string {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		runtime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtime, "z13gui-frozen-pid")
}

// FindSteamPID locates the main Steam process by scanning /proc.
// Returns 0 if not found (graceful degradation — no freeze attempted).
func FindSteamPID() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "steam" {
			slog.Debug("steam: found process", "pid", pid)
			return pid
		}
	}
	return 0
}

// FreezeProc sends SIGSTOP to suspend a process and writes the PID to a
// state file for crash recovery.
func FreezeProc(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		return err
	}
	_ = os.WriteFile(pidFilePath(), []byte(strconv.Itoa(pid)), 0o600)
	return nil
}

// ThawProc sends SIGCONT to resume a process and removes the state file.
func ThawProc(pid int) error {
	_ = os.Remove(pidFilePath())
	return syscall.Kill(pid, syscall.SIGCONT)
}

// ThawFrozen reads the PID state file and sends SIGCONT if a frozen PID is
// recorded. Used by signal handlers and cleanup paths for crash recovery.
func ThawFrozen() {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGCONT)
	_ = os.Remove(pidFilePath())
	slog.Info("steam: thawed frozen process on cleanup", "pid", pid)
}
