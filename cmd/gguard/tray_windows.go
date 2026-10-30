//go:build windows

package main

import (
	"time"

	"fyne.io/systray"
)

// trayOptions carries what the icon needs. It is a *view* of the runner: the
// routines live in another process, so everything the menu does — showing the
// states, running, restarting, stopping — goes through the dashboard API the
// runner already serves. That is what makes closing the icon harmless.
type trayOptions struct {
	// runnerURL is where the runner answers, or "" when it is not up.
	runnerURL func() string
	logPath   string
	configDir string
	// start brings the runner up when it is not running.
	start func()
}

// trayAvailable reports whether a system tray can be shown on this platform.
func trayAvailable() bool { return true }

// runTray shows the tray icon and blocks until the user closes it. It must be
// called from the main goroutine.
func runTray(opts trayOptions) {
	systray.Run(func() {
		menu := &trayMenu{opts: opts}
		menu.build()
		menu.refresh()

		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				menu.refresh()
			}
		}()
	}, func() {})
}

// trayMenu is the menu currently on screen. It is thrown away and built again
// whenever the routines or the runner change, because systray only appends:
// rebuilding is what keeps the order stable.
type trayMenu struct {
	opts       trayOptions
	online     bool
	guards     []statusEntry
	items      []*systray.MenuItem
	guardItems map[string]*systray.MenuItem
	header     *systray.MenuItem
}

// refresh pulls the current state from the runner and redraws what changed.
func (m *trayMenu) refresh() {
	url := m.opts.runnerURL()
	state, online := fetchState(url)
	if online != m.online || !sameGuards(state.Guards, m.guards) {
		m.online = online
		m.guards = state.Guards
		m.build()
	}

	label := "not running"
	iconState := StateIdle
	if online {
		label = state.Label
		iconState = stateFromString(state.Overall)
	}

	systray.SetIcon(trayIconICO(iconState))
	systray.SetTooltip("GopherGuard — " + label)
	if m.header != nil {
		m.header.SetTitle("GopherGuard — " + label)
	}

	for name, item := range m.guardItems {
		item.SetIcon(statusDotICO(stateFromString(stateOf(state.Guards, name))))
	}
}

// build drops the current menu and creates it again from scratch, starting the
// click handlers of the new items.
func (m *trayMenu) build() {
	for _, item := range m.items {
		item.Remove()
	}
	m.items = nil
	m.guardItems = make(map[string]*systray.MenuItem)

	title := "GopherGuard — not running"
	if m.online {
		title = "GopherGuard"
	}
	m.header = m.add(systray.AddMenuItem(title, "Current status"))
	m.header.Disable()

	if !m.online {
		systray.AddSeparator()
		m.onClick(m.add(systray.AddMenuItem("Start GopherGuard", "Start the process that runs the routines")), func() {
			if m.opts.start != nil {
				go m.opts.start()
			}
		})
		systray.AddSeparator()
		m.onClick(m.add(systray.AddMenuItem("Quit", "Close the tray icon")), systray.Quit)
		return
	}

	// One entry per routine, each with its own colored dot.
	if len(m.guards) > 0 {
		systray.AddSeparator()
		for _, guard := range m.guards {
			item := systray.AddMenuItem(guard.Name, "Run this routine now")
			item.SetIcon(statusDotICO(stateFromString(guard.State)))
			m.guardItems[guard.Name] = item
			m.items = append(m.items, item)

			name := guard.Name
			m.onClick(item, func() { m.post("/api/run", `{"name":"`+name+`"}`) })
		}
	}

	systray.AddSeparator()
	m.onClick(m.add(systray.AddMenuItem("Open dashboard", "Open the web dashboard in your browser")), func() {
		openURL(m.opts.runnerURL())
	})
	m.onClick(m.add(systray.AddMenuItem("Run all now", "Run every routine immediately")), func() {
		m.post("/api/run", "{}")
	})
	m.onClick(m.add(systray.AddMenuItem("Open log", "Open gguard.log")), func() {
		openPath(m.opts.logPath)
	})
	m.onClick(m.add(systray.AddMenuItem("Open folder", "Open the install folder")), func() {
		openPath(m.opts.configDir)
	})

	systray.AddSeparator()
	m.onClick(m.add(systray.AddMenuItem("Stop Process", "Stop the routines, keeping the tray")), func() {
		m.post("/api/stop", "{}")
		time.Sleep(300 * time.Millisecond)
		m.refresh()
	})
	m.onClick(m.add(systray.AddMenuItem("Restart Process", "Read the .ggs scripts and guards.json again and start them")), func() {
		m.post("/api/reload", "{}")
		time.Sleep(300 * time.Millisecond)
		m.refresh()
	})
	m.onClick(m.add(systray.AddMenuItem("Stop and Quit", "Stop the routines and close the tray")), func() {
		m.post("/api/quit", "{}")
		systray.Quit()
	})

	systray.AddSeparator()
	m.onClick(m.add(systray.AddMenuItem("Quit", "Close the tray icon; the routines keep running")), systray.Quit)
}

// post sends one action to the runner's API, which is the only thing the icon
// can do: it does not own the routines.
func (m *trayMenu) post(path, body string) {
	postAPI(m.opts.runnerURL(), path, body)
}

func (m *trayMenu) add(item *systray.MenuItem) *systray.MenuItem {
	m.items = append(m.items, item)
	return item
}

// onClick runs fn on every click, and ends when the item is removed.
func (m *trayMenu) onClick(item *systray.MenuItem, fn func()) {
	go func() {
		for range item.ClickedCh {
			fn()
		}
	}()
}

func sameGuards(a, b []statusEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func stateOf(guards []statusEntry, name string) string {
	for _, g := range guards {
		if g.Name == name {
			return g.State
		}
	}
	return ""
}
