// Package notify shows desktop notifications in a best-effort way. Failures are
// reported to the caller but are never fatal.
package notify

// enabled allows callers (mainly tests) to silence notifications globally.
var enabled = true

// SetEnabled toggles notifications globally.
func SetEnabled(v bool) { enabled = v }

// Enabled reports whether notifications are on.
func Enabled() bool { return enabled }
