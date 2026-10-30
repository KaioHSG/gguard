//go:build !windows

package main

// payloadFiles lists the binaries installed on this platform, in copy order.
// Outside Windows a single binary is enough: there is no console window to
// avoid.
func payloadFiles() []string {
	return []string{"gguard"}
}
