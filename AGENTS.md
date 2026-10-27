# AGENTS.md — gguard

## Build & Test

```bash
# Dev build for the current OS (Windows: gguard.exe, Linux/macOS: gguard)
go run build.go

# Dist — all platforms (Windows x64/ARM, Linux x64/ARM) into dist/
go run build.go --dist
```

`build.go` is a `//go:build ignore` helper run with `go run`, replacing the old
`build.ps1`/`build.sh`. It needs `go-winres` on PATH to stamp Windows resources.

## Lint & Test

```bash
go vet ./...
go test ./...
```

Run `go build ./cmd/gguard`, `go vet ./...` and `go test ./...` to verify changes.

## Project Structure

```
cmd/gguard/          — main package (the CLI binary)
  main.go            — CLI subcommand dispatch (install/start/status/import/...), flag parsing,
                       runGuards(), config loading, status file, targetMode(), doImport(), doExport()
  main_windows.go    — isServiceManager(), isTerminal(), startElevated(), showInstallDialog(),
                       pauseBeforeExit(), activeUserConfigDir() (WTS API), userConfigDir()
  main_other.go      — same as main_windows.go but non-Windows (pkexec/sudo, isServiceManager=false)
  service.go         — program struct (service.Interface), serviceControl(), logging
  install.go         — doInstall(), doUninstall(), doUpgrade(), detectInstallType(), install dir removal
  install_windows.go — Windows: install dirs, PATH registry, Start Menu shortcuts, admin check, Apps & Features entry
  install_other.go   — Linux/macOS: desktop entry, icon, install dirs (PATH/registry no-op)
  icon.go            — tray/app icon rendering (embeds winres/icons/)
  state.go           — runtime state/status file
  tray*.go           — system tray UI (Windows/non-Windows, systray)
  gui.go             — loopback dashboard (doGUI, startDashboard, log ring, tray integration)
  open.go            — openURL()/openPath(): default browser/handler per platform
  resources.go       — embeds the resources/ directory
  resources/         — embedded assets (gguard.desktop, gguard.png, dashboard.html)
  winres/            — go-winres config + icons (winres.json, icons/)
  icon_test.go       — icon rendering tests
build.go             — cross-platform build script (go run build.go [--dist]); replaces build.ps1/build.sh
engine/              — File watcher engine (watcher, dispatcher, debouncer, engine)
ggs/                 — .ggs script parser (AST, parser)
notify/              — platform notifications
example/             — sample .ggs / guards.json
img/                 — README images
docs/                — GG-Script.md, Guards-JSON.md documentation
```

`resources/` and `winres/` live inside `cmd/gguard/` because `go:embed` cannot reference paths outside the package directory.

## Platform-specific Files (Go Build Tags)

| File | Tag |
|------|-----|
| `cmd/gguard/main_windows.go` | `//go:build windows` |
| `cmd/gguard/install_windows.go` | `//go:build windows` |
| `cmd/gguard/main_other.go` | `//go:build !windows` |
| `cmd/gguard/install_other.go` | `//go:build !windows` |

## Key Conventions

- CLI: subcommands (`install`, `uninstall`, `upgrade`, `start`, `stop`, `restart`, `status`, `import <file>`, `export`, `version`, `help`); flags are accepted before or after the subcommand.
- Flags: double-dash `--flag` normalized to `-flag` in main() via os.Args mutation.
- Running as service manager (`isServiceManager()`) takes precedence over subcommands; a bare interactive run with no flags shows the install dialog via `showInstallDialog()`.
- `dispatch()` routes subcommands; `targetMode(userFlag)` decides system vs user for service/import/export.
- Service names: `"gguard"` (system) vs `"gguard-user"` (user).
- `isAdmin()`: checks token group membership (Windows) / euid (Linux).
- `activeUserConfigDir()`: returns the logged-in console user's config dir (Windows via WTS API).
- `userConfigDir()`: returns the current process user's config dir (same as `installTargetDir(true)`).
- `targetMode()`: auto-detects system/user based on admin status; `--user` always overrides to user.
- `serviceControl()` returns `error` instead of calling log.Fatalf (callers decide).
- `runForeground(ctx, cancel, foregroundOptions)` is the only foreground entry point: guards + tray (when the tray setting is on, or forced by `gguard tray`) + dashboard (`gguard gui`, or whenever the tray is up). It blocks until the guards stop.
- The tray is a **setting**, not a command: `trayEnabled()` reads the per-user logon entry written by `enableTrayAutostart()` and removed by `disableTrayAutostart()`. `doInstall()` enables it only for a fresh install, so an upgrade never overrides `tray --off`.
- Launcher-started processes (double-click, Start Menu shortcut, logon entry) call `hideOwnConsole()` and report errors through `fatalUserError()`, which uses a message box when the console is gone.
- `doGUI()` opens the existing dashboard when one already answers on `guiDefaultPort` (8642), so only one process ever runs the guards; `quitDashboard()` lets `tray --off` stop an instance with no usable pid file.
- `teeLogs()` mirrors the process log into a ring buffer so the dashboard shows it even when the console is gone; `logTee` adds `gguard.log` to the same output in quiet mode.
- `trayPidPath()` lives in the user cache dir, because a system install directory (Program Files) is not writable by the user running the tray.
- PATH modification uses `SetExpandStringValue` for system PATH / when original was REG_EXPAND_SZ.
- `doImport()`: copies `.ggs` or merges `guards.json` (replace total per routine key).
- `doExport()`: lists all `.ggs` and `guards.json` paths for the current mode.