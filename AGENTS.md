# AGENTS.md — gguard

## Build & Test

```bash
# Windows (dev build)
go build -ldflags "-s -w" -o gguard.exe .

# Windows (dist — all platforms)
./build.ps1 -Dist

# Linux/macOS (dev build)
go build -ldflags "-s -w" -o gguard .

# Linux/macOS (dist — all platforms)
./build.sh --dist
```

## Lint & Vet

```bash
go vet ./...
```

No test suite exists yet. Run `go build .` and `go vet ./...` to verify changes.

## Project Structure

```
main.go              — CLI flag parsing, runGuards(), config loading, status file,
                     — targetMode(), doImport(), doExport()
main_windows.go      — isServiceManager(), activeUserConfigDir() (WTS API), userConfigDir()
main_other.go        — isServiceManager() (always false), activeUserConfigDir(), userConfigDir()
service.go           — program struct (service.Interface), serviceControl(), logging
install.go           — doInstall(), doUninstall(), doUpgrade(), shared install logic
install_windows.go   — Windows: install dirs, PATH registry, Start Menu shortcuts, admin check
install_other.go     — Linux/macOS: desktop entry, icon, install dirs (PATH no-op)
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

- Flags: double-dash `--flag` normalized to `-flag` in main() via os.Args mutation.
- Service names: `"gguard"` (system) vs `"gguard-user"` (user).
- `isAdmin()`: checks token group membership (Windows) / euid (Linux).
- `activeUserConfigDir()`: returns the logged-in console user's config dir (Windows via WTS API).
- `userConfigDir()`: returns the current process user's config dir (same as `installTargetDir(true)`).
- `targetMode()`: auto-detects system/user based on admin status; `--user` always overrides to user.
- `serviceControl()` returns `error` instead of calling log.Fatalf (callers decide).
- PATH modification uses `SetExpandStringValue` for system PATH / when original was REG_EXPAND_SZ.
- `doImport()`: copies `.ggs` or merges `guards.json` (replace total per routine key).
- `doExport()`: lists all `.ggs` and `guards.json` paths for the current mode.