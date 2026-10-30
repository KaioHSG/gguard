//go:build windows

package main

// payloadFiles lists the binaries installed on this platform, in copy order.
// gguard.exe is the app, a normal console program; gguard-launcher.exe is the
// windowless launcher the logon entry and the Start Menu shortcut use, so they
// never flash a console window.
func payloadFiles() []string {
	return []string{"gguard.exe", "gguard-launcher.exe"}
}
