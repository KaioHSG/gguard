//go:build !windows

package main

import "errors"

var errTrayUnsupported = errors.New("tray process control is not supported on this platform")

// serviceRunning reports whether a Windows service is running. Always unknown
// off Windows; the tray is unavailable there anyway.
func serviceRunning(_ string) (running bool, known bool) { return false, false }

func spawnDetached(_ string, _ []string) (int, error) { return 0, errTrayUnsupported }

func processAlive(_ int) bool { return false }

func killProcess(_ int) error { return errTrayUnsupported }
