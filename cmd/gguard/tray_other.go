//go:build !windows

package main

// trayOptions carries everything the tray needs to drive the running guards.
// On platforms without a tray implementation it is unused.
type trayOptions struct {
	tracker   *StatusTracker
	runAll    func()
	runOne    func(string)
	logPath   string
	configDir string
	onQuit    func()
	status    func() (GuardState, string)
	// dashboardURL is unused off Windows (no tray), but kept so the struct
	// matches the one the tray implementation expects.
	dashboardURL string
}

// trayAvailable reports whether a system tray can be shown on this platform.
func trayAvailable() bool { return false }

// quitTray is a no-op off Windows (there is no tray to close).
func quitTray() {}

// runTray is a no-op on platforms without a tray. It is never reached because
// trayAvailable returns false, but it keeps the build compiling everywhere.
func runTray(_ trayOptions) {}
