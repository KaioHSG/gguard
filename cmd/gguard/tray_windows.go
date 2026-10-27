//go:build windows

package main

import (
	"os/exec"
	"time"

	"fyne.io/systray"
)

// trayOptions carries everything the tray needs to drive the running guards.
type trayOptions struct {
	tracker   *StatusTracker
	runAll    func()
	runOne    func(string)
	logPath   string
	configDir string
	onQuit    func()
	// status returns the overall state and a short label to display. When no
	// guards are loaded it reports the service state instead.
	status func() (GuardState, string)
}

// trayAvailable reports whether a system tray can be shown on this platform.
func trayAvailable() bool { return true }

// runTray shows the tray icon and blocks until the user quits it. It must be
// called from the main goroutine.
func runTray(opts trayOptions) {
	systray.Run(func() {
		header := systray.AddMenuItem("GopherGuard", "Current status")
		header.Disable()

		// One entry per guard, each with its own colored dot.
		names := opts.tracker.Names()
		guardItems := make(map[string]*systray.MenuItem, len(names))
		if len(names) > 0 {
			systray.AddSeparator()
			for _, name := range names {
				item := systray.AddMenuItem(name, "Run this guard now")
				item.SetIcon(statusDotICO(opts.tracker.State(name)))
				guardItems[name] = item
			}
		}

		systray.AddSeparator()
		runItem := systray.AddMenuItem("Run all now", "Run every guard immediately")
		logItem := systray.AddMenuItem("Open log", "Open gguard.log")
		dirItem := systray.AddMenuItem("Open folder", "Open the install folder")
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Quit", "Stop GopherGuard")

		refresh := func() {
			state := StateIdle
			label := "idle"
			if opts.status != nil {
				state, label = opts.status()
			}
			systray.SetIcon(trayIconICO(state))
			systray.SetTooltip("GopherGuard — " + label)
			header.SetTitle("GopherGuard — " + label)

			for name, item := range guardItems {
				item.SetIcon(statusDotICO(opts.tracker.State(name)))
			}
		}

		opts.tracker.AddListener(refresh)
		refresh()

		// Poll so the icon also tracks the service state when there are no
		// local guards.
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				refresh()
			}
		}()

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

		// Per-guard click handlers.
		for name, item := range guardItems {
			go func(name string, item *systray.MenuItem) {
				for range item.ClickedCh {
					if opts.runOne != nil {
						go opts.runOne(name)
					}
				}
			}(name, item)
		}
	}, func() {})
}

// openPath opens a file or folder with the default Windows handler.
func openPath(path string) {
	if path == "" {
		return
	}
	_ = exec.Command("cmd", "/c", "start", "", path).Start()
}
