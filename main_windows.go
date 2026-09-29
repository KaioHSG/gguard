//go:build windows

package main

import (
	"golang.org/x/sys/windows/svc"
)

func isServiceManager() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}