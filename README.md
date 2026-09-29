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

### 5. Installation & Usage

```
# Download and run
gguard -s example/backup.ggs -g example/guards.json

# Install as a system service (copies to standard location + adds to PATH)
gguard --install
gguard --start
gguard --status

# Or install as a per-user service (Linux: adds .desktop entry + icon)
gguard --install --user
gguard --start
gguard --status
```

On Linux, `--install` also registers a `.desktop` entry and icon, so GopherGuard appears in your application launcher.

### 6. Context Variables

| Variable | Resolves to |
|----------|-------------|
| `{home}` | User's home directory |
| `{watch}` | Monitored path |
| `{dest}` | Backup/sync destination (from `guards.json`) |
| `{timestamp}` | Current date/time (`2026-09-23_12-30-00`) |
| `{file}` | Name of the file that triggered the event |

### 7. Available Commands

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

### 8. guards.json Settings

| Field | Description |
|-------|-------------|
| `dest` | Destination folder |
| `trusted` | Allows sensitive commands (backup cleanup) |
| `autostart` | Starts automatically when the service is running |
| `free_cache` | Releases OneDrive cache after (e.g., `"1h"`) |
| `keep_backups` | Max zips kept (requires `trusted`) |
| `delete_older_than` | Removes zips older than (e.g., `"7d"`) |