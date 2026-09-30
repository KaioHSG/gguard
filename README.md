# GopherGuard (gguard)

*Lightweight, cross-platform file watcher and backup automator written in Go. Monitors directory changes and triggers configurable tasks — ZIP compression, incremental sync, and more.*

### 1. Architecture

Three loosely coupled modules:

- **Filesystem Watcher (fsnotify)**: Listens for OS events (create, modify, delete) on specified paths.
- **Debouncer / Timer Engine**: Smart filter. Applications often save files in bursts (multiple events per second). The engine waits for a "silence" period (e.g., 5 seconds with no changes) before releasing the task execution.
- **Task Dispatcher**: Executor. A queue of customizable tasks that receives the validated trigger and runs the defined routine (e.g., compress to .zip, send to OneDrive).

### 2. Tech Stack (Go)

- **Go**: Single static binary, low RAM usage, excellent concurrency support (goroutines).
- **fsnotify**: Standard library for cross-platform file event monitoring (ReadDirectoryChangesW on Windows, inotify on Linux).
- **Configuration**: `.ggs` scripts (declarative DSL) + `guards.json` (per-routine private configuration).

### 3. Execution Flow

1. **Initialization**: The binary reads the `.ggs` script and `guards.json` with the monitoring rules.
2. **Watcher Registration**: For the target directory, Go starts a goroutine using fsnotify (recursive subdirectory support).
3. **Event Capture**: The filesystem fires modification events (WRITE / CREATE).
4. **Debounce**: The event cancels the previous timer and restarts the count. If new events arrive, the timer is reset. When the timer hits zero (end of the change burst), the consolidated event is sent to the task queue.
5. **Task Dispatch**: The Dispatcher executes the configured actions (e.g., zip, log message) with resolved variables.

### 4. Script Example (.ggs)

```nidx
# Backup routine

guard "My Backup"
version 1.0
os windows

watch "{home}/Documents/Projetos"
debounce 5s
zip {dest}/projetos-{timestamp}.zip {watch}
message "Backup saved at \"{dest}\""
```

### 5. Installation

#### System-wide (requires admin)

```powershell
# Windows (Run as Administrator)
.\gguard.exe --install
.\gguard.exe --start
.\gguard.exe --status
.\gguard.exe --stop
```

```bash
# Linux (with sudo)
sudo ./gguard --install
sudo ./gguard --start
sudo ./gguard --status
sudo ./gguard --stop
```

Installs to:
- **Windows**: `%ProgramFiles%\GopherGuard\gguard.exe`
- **Linux**: `/usr/local/bin/gguard`

Adds to system PATH, registers as a system service (`gguard`), creates Start Menu shortcut (Windows) / desktop entry (Linux).

#### Per-user (no admin required)

```powershell
.\gguard.exe --install --user
.\gguard.exe --start --user
```

Installs to:
- **Windows**: `%APPDATA%\Programs\GopherGuard\gguard.exe`
- **Linux**: `~/.local/bin/gguard`

Registers as a user service (`gguard-user`), runs under your account while logged in.

### 6. Service Management (Auto-detect Mode)

Commands auto-detect whether you need the system or user service based on your privileges:

| Command | Without `--user` | With `--user` |
|---------|------------------|---------------|
| `--install` | **system** (needs admin) | **user** (no admin) |
| `--start` | **system** (needs admin) | **user** |
| `--stop` | **system** (needs admin) | **user** |
| `--restart` | **system** (needs admin) | **user** |
| `--status` | **system** (no admin needed) | **user** |
| `--import` | **system** (if admin) / **user** (if not) | **user** |
| `--export` | **system** (if admin) / **user** (if not) | **user** |

```
# As a regular user — operates on your user service
gguard --status

# As admin — operates on system service
sudo gguard --status

# Explicit user override (even if running as admin)
sudo gguard --status --user
```

### 7. Multi-User Config (System Service)

When running as a **system service**, gguard automatically detects who is logged in at the console and loads their personal config on top of the global config:

```
Config layers (system service):

  Global (admin only):
    %ProgramFiles%\GopherGuard\gg-scripts\*.ggs
    %ProgramFiles%\GopherGuard\guards.json

  Current user (no admin needed):
    %APPDATA%\Programs\GopherGuard\gg-scripts\*.ggs
    %APPDATA%\Programs\GopherGuard\guards.json
```

- The admin places **global** scripts everyone should run
- Each user places their **personal** scripts in their own AppData (no admin required)
- The system service (running as SYSTEM) loads and runs both

### 8. Import / Export

```powershell
# Import a .ggs script (copies to the correct directory)
gguard --import backup.ggs

# Import a guards.json (merges with existing, never loses existing keys)
gguard --import config.json

# Export current scripts and config (lists paths)
gguard --export
```

The target directory depends on mode:
- **System mode** (admin): `%ProgramFiles%\GopherGuard`
- **User mode** (non-admin or `--user`): `%APPDATA%\Programs\GopherGuard`

Import merge behavior for `guards.json`:
- Existing keys not present in the import are **preserved**
- Keys present in the import are **fully replaced**
- Never overwrites the entire file

### 9. Context Variables

| Variable | Resolves to |
|----------|-------------|
| `{home}` | User's home directory |
| `{watch}` | Monitored path |
| `{dest}` | Backup/sync destination (from `guards.json`) |
| `{timestamp}` | Current date/time (`2026-09-23_12-30-00`) |
| `{file}` | Name of the file that triggered the event |

### 10. Available Commands

| Command | Description |
|---------|-------------|
| `guard` | Routine name (required) |
| `version` | Routine version (required) |
| `os` | Target system: `windows`, `linux`, `darwin` |
| `watch` | Directory to monitor (absolute path or with `{home}`) |
| `debounce` | Silence time after the last event (e.g., `5s`, `10s`, `1m`) |
| `zip` | Compresses `{source}` into `{destination}.zip` |
| `sync` | Syncs `{source}` → `{destination}` (incremental) |
| `sync --delete` | Sync + removes orphans (requires `trusted`) |
| `sync --bidir` | Bidirectional sync (copies both ways) |
| `sync --bidir --delete` | Bidirectional sync with orphan removal |
| `message` | Post-execution log / notification |

### 11. guards.json Settings

| Field | Description |
|-------|-------------|
| `dest` | Destination folder |
| `trusted` | Allows sensitive commands (backup cleanup) |
| `autostart` | Starts automatically when the service is running |
| `free_cache` | Releases OneDrive cache after (e.g., `"1h"`) |
| `keep_backups` | Max zips kept (requires `trusted`) |
| `delete_older_than` | Removes zips older than (e.g., `"7d"`) |

### 12. Logging

When running as a service, logs are written to `gguard.log` in the install directory:
- **System**: `%ProgramFiles%\GopherGuard\gguard.log`
- **User**: `%APPDATA%\Programs\GopherGuard\gguard.log`

Use `-q` or `--quiet` to force log-to-file in foreground mode.