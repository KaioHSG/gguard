//go:build !windows

package main

func isServiceManager() bool {
	return false
}