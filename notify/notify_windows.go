//go:build windows

// Package notify shows desktop notifications in a best-effort way. Failures are
// reported to the caller but are never fatal.
package notify

import (
	"os"
	"os/exec"
)

// toastScript renders a Windows 10+ toast using the WinRT notification APIs.
// The title and body are passed through environment variables to avoid any
// quoting problems.
const toastScript = `$ErrorActionPreference='Stop'
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
[void][Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType=WindowsRuntime]
$t=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$texts=$t.GetElementsByTagName('text')
[void]$texts.Item(0).AppendChild($t.CreateTextNode($env:GGUARD_TOAST_TITLE))
[void]$texts.Item(1).AppendChild($t.CreateTextNode($env:GGUARD_TOAST_MSG))
$toast=[Windows.UI.Notifications.ToastNotification]::new($t)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('GopherGuard').Show($toast)`

// Notify displays a desktop notification.
func Notify(title, message string) error {
	if !enabled {
		return nil
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", toastScript)
	cmd.Env = append(os.Environ(),
		"GGUARD_TOAST_TITLE="+title,
		"GGUARD_TOAST_MSG="+message,
	)
	return cmd.Run()
}
