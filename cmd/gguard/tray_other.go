//go:build !windows

package main

// trayOptions carries what the icon needs. It is a *view* of the runner: the
// routines live in another process and are reached through its dashboard API.
type trayOptions struct {
	runnerURL func() string
	logPath   string
	configDir string
	start     func()
}

// trayAvailable reports whether a system tray can be shown on this platform.
func trayAvailable() bool { return false }

// runTray is a no-op on platforms without a tray. It is never reached because
// trayAvailable returns false, but it keeps the build compiling everywhere.
func runTray(_ trayOptions) {}
