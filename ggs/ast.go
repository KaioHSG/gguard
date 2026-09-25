package ggs

import "time"

type Guard struct {
	Name     string
	Version  string
	OS       string
	Watch    string
	Debounce time.Duration
	Commands []Command
}

type Command struct {
	Verb string
	Args []string
}