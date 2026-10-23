package ggs

import (
	"strings"
	"time"
)

type Guard struct {
	Name     string
	Version  string
	OS       string
	Watch    string
	Debounce time.Duration
	Commands []Command
}

// MatchesOS reports whether the guard should run on the given OS. An empty os
// header matches every platform; otherwise it accepts a comma-separated list
// (e.g. "windows,linux").
func (g *Guard) MatchesOS(current string) bool {
	os := strings.TrimSpace(g.OS)
	if os == "" {
		return true
	}
	for _, part := range strings.Split(os, ",") {
		if strings.EqualFold(strings.TrimSpace(part), current) {
			return true
		}
	}
	return false
}

type Command struct {
	Verb string
	Args []string
}
