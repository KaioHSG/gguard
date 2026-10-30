// Command gguard-launcher starts gguard without a console window.
//
// It does one thing: hand its own arguments to gguard, detached, and return.
// That is all the GUI entry points need — the logon entry, the Start Menu
// shortcut, a button that requires a new process — and it is why this binary,
// and not the app, is built without a console (-H windowsgui).
//
// gguard itself stays a normal console program, so "gguard status" works in a
// terminal. Use the app for anything you type.
package main

import (
	"fmt"
	"os"
)

func main() {
	app, err := appPath()
	if err != nil {
		notify(err.Error())
		os.Exit(1)
	}

	if err := startDetached(app, os.Args[1:]); err != nil {
		notify(fmt.Sprintf("cannot start gguard: %v", err))
		os.Exit(1)
	}
}
