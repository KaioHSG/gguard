package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// payloadFS carries the binaries this installer writes: gguard and, on
// Windows, gguard-launcher. build.go fills payload/ before building the
// installer, so a single download is enough to install GopherGuard.
//
//go:embed payload/*
var payloadFS embed.FS

// payloadPlaceholder only exists so the package compiles before build.go has
// generated the binaries. It is never installed.
const payloadPlaceholder = "placeholder.txt"

// installPayload writes every embedded binary into dir, replacing whatever is
// there.
func installPayload(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	for _, name := range payloadFiles() {
		data, err := payloadFS.ReadFile("payload/" + name)
		if err != nil {
			return fmt.Errorf("this build has no %s to install", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0755); err != nil {
			return fmt.Errorf("cannot write %s: %w", name, err)
		}
		fmt.Printf("installed %s\n", name)
	}
	return nil
}
