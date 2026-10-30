package install

import "embed"

// resources holds the desktop entry and the icon written on Linux and macOS.
//
//go:embed resources/*
var resources embed.FS
