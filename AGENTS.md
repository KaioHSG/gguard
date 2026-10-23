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

## Lint & Vet

```bash
go vet ./...
```

No test suite exists yet. Run `go build .` and `go vet ./...` to verify changes.

## Project Structure

```
main.go              — CLI subcommand dispatch (install/start/status/import/...), flag parsing,
                       runGuards(), config loading, status file, targetMode(), doImport(), doExport()
main_windows.go      — isServiceManager(), isTerminal(), startElevated(), showInstallDialog(),
                       pauseBeforeExit(), activeUserConfigDir() (WTS API), userConfigDir()
main_other.go        — same as main_windows.go but non-Windows (pkexec/sudo, isServiceManager=false)
service.go           — program struct (service.Interface), serviceControl(), logging
install.go           — doInstall(), doUninstall(), doUpgrade(), detectInstallType(), install dir removal
install_windows.go   — Windows: install dirs, PATH registry, Start Menu shortcuts, admin check, Apps & Features entry
install_other.go     — Linux/macOS: desktop entry, icon, install dirs (PATH/registry no-op)
build.go             — cross-platform build script (go run build.go [--dist]); replaces build.ps1/build.sh
resources.go         — embeds resources/ directory
engine/              — File watcher engine (watcher, dispatcher, debouncer, engine)
ggs/                 — .ggs script parser (AST, parser)
```

## Platform-specific Files (Go Build Tags)

| File | Tag |
|------|-----|
| `main_windows.go` | `//go:build windows` |
| `install_windows.go` | `//go:build windows` |
| `main_other.go` | `//go:build !windows` |
| `install_other.go` | `//go:build !windows` |

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
- PATH modification uses `SetExpandStringValue` for system PATH / when original was REG_EXPAND_SZ.
- `doImport()`: copies `.ggs` or merges `guards.json` (replace total per routine key).
- `doExport()`: lists all `.ggs` and `guards.json` paths for the current mode.