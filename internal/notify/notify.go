// Package notify shows a desktop notification, best effort: a failure is ignored, since every
// warning is also shown where the user is looking (Claude's status line, the tray, hat usage).
package notify

import (
	"os/exec"
	"runtime"
	"strings"
)

// Show starts the OS's notifier and returns without waiting for it, so a status line is not
// held up. It reports whether a notifier was found.
func Show(title, body string) bool {
	cmd := command(runtime.GOOS, title, body)
	if cmd == nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	go cmd.Wait()
	return true
}

func command(goos, title, body string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("osascript", "-e", "display notification "+appleString(body)+" with title "+appleString(title))
	case "windows":
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", toast(title, body))
	}
	if p, err := exec.LookPath("notify-send"); err == nil {
		return exec.Command(p, "--app-name=hatrack", title, body)
	}
	if p, err := exec.LookPath("powershell.exe"); err == nil { // WSL: notify on the Windows desktop
		return exec.Command(p, "-NoProfile", "-NonInteractive", "-Command", toast(title, body))
	}
	return nil
}

// appleString quotes s for AppleScript.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// psString quotes s for PowerShell.
func psString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// toast shows a Windows notification through the WinRT API, attributed to PowerShell, whose app
// id Windows already knows, so nothing has to be registered.
func toast(title, body string) string {
	return `$ErrorActionPreference='Stop';` +
		`[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]>$null;` +
		`$x=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);` +
		`$t=$x.GetElementsByTagName('text');` +
		`$t.Item(0).AppendChild($x.CreateTextNode(` + psString(title) + `))>$null;` +
		`$t.Item(1).AppendChild($x.CreateTextNode(` + psString(body) + `))>$null;` +
		`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($x))`
}
