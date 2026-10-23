//go:build windows

package main

import (
	"os/exec"

	"fyne.io/systray"
)

// trayOptions carries everything the tray needs to drive the running guards.
type trayOptions struct {
	tracker   *StatusTracker
	runAll    func()
	logPath   string
	configDir string
	onQuit    func()
}

// trayAvailable reports whether a system tray can be shown on this platform.
func trayAvailable() bool { return true }

// runTray shows the tray icon and blocks until the user quits it. It must be
// called from the main goroutine.
func runTray(opts trayOptions) {
	systray.Run(func() {
		status := systray.AddMenuItem("GopherGuard", "Current status")
		status.Disable()
		runItem := systray.AddMenuItem("Run now", "Run all guards immediately")
		systray.AddSeparator()
		logItem := systray.AddMenuItem("Open log", "Open gguard.log")
		dirItem := systray.AddMenuItem("Open folder", "Open the install folder")
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Quit", "Stop GopherGuard")

		refresh := func() {
			st := opts.tracker.Aggregate()
			systray.SetIcon(statusDotICO(st))
			systray.SetTooltip("GopherGuard — " + st.String())
			status.SetTitle("Status: " + st.String())
		}
		opts.tracker.AddListener(refresh)
		refresh()

		go func() {
			for {
				select {
				case <-runItem.ClickedCh:
					if opts.runAll != nil {
						go opts.runAll()
					}
				case <-logItem.ClickedCh:
					openPath(opts.logPath)
				case <-dirItem.ClickedCh:
					openPath(opts.configDir)
				case <-quitItem.ClickedCh:
					if opts.onQuit != nil {
						opts.onQuit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}

// openPath opens a file or folder with the default Windows handler.
func openPath(path string) {
	if path == "" {
		return
	}
	_ = exec.Command("cmd", "/c", "start", "", path).Start()
}
