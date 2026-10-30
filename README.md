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

Installing is the job of **`gguard-setup`**, which carries the binaries it
writes, so a single download is enough. Double-click it for a menu, or use it
from a terminal. The release files **are** the installer, named for the
platform (the examples below call it `gguard-setup`):

| Release file | Platform |
|--------------|----------|
| `GopherGuard-Windows-x64.exe` | Windows 64-bit |
| `GopherGuard-Windows-ARM.exe` | Windows on ARM |
| `GopherGuard-Linux-x64` | Linux 64-bit |
| `GopherGuard-Linux-ARM` | Linux on ARM |

On Linux, mark it executable once: `chmod +x GopherGuard-Linux-x64`.

#### System-wide (requires admin)

```powershell
# Windows (Run as Administrator)
.\gguard-setup.exe install
```

```bash
# Linux (with sudo)
sudo ./gguard-setup install
```

Installs to:
- **Windows**: `%ProgramFiles%\GopherGuard\`
- **Linux**: `/usr/local/bin/gguard`

Adds to system PATH, registers as a system service (`gguard`), turns the tray on (`gguard tray --off` to disable), and creates a Start Menu shortcut (Windows) / desktop entry (Linux) that opens the **dashboard**.

#### Per-user (no admin required)

```bash
./gguard-setup install --user
```

Installs to:
- **Windows**: `%APPDATA%\Programs\GopherGuard\`
- **Linux**: `~/.local/bin/gguard`

Registers as a user service (`gguard-user`), runs under your account while logged in.

#### The three binaries

| File | What it is |
|------|------------|
| `gguard` | The app: guards, service, tray, dashboard, `uninstall` — and a normal console program, so `gguard status` works in a terminal |
| `gguard-launcher` | **Windows only.** A windowless launcher that starts `gguard` with the same arguments and returns immediately |
| `gguard-setup` | The installer: `install`, `upgrade`, `uninstall`, and the double-click menu. It runs from where you downloaded it — it is **not** copied into the install directory |

The launcher is built **without a console** (`-H=windowsgui`), and that is the
point of it: anything Windows starts on its own — the logon entry, the Start
Menu shortcut, a GUI button that needs a new process — goes through the
launcher, which hands its arguments to `gguard`, detaches and leaves. No
console ever appears.

That frees `gguard` to stay a normal console program, so the terminal is
unchanged:

```powershell
gguard status
gguard gui
```

#### Upgrading and uninstalling

```bash
gguard-setup upgrade      # keeps the same install type and the tray setting
gguard uninstall          # the app removes itself, its service and its shortcuts
```

Uninstalling lives in **the app**, not the installer: the app is what the
install directory and the Apps & Features entry point at, so it keeps working
long after the original download is gone. The install directory therefore holds
`gguard` and `gguard-launcher` and nothing else.

On Windows a running file cannot delete itself, so the app asks the system to
finish removing the install directory on the next restart.

### 6. Service Management (Auto-detect Mode)

`start`, `stop`, `restart` and `status` first detect **which service is actually installed** (system `gguard` or user `gguard-user`), so they keep working regardless of your current privileges. When the command needs elevation for a system-wide install, gguard prints a hint to re-run as administrator (Windows) or with `sudo` (Linux).

The remaining commands auto-detect the system/user service based on your privileges:

| Command | Without `--user` | With `--user` |
|---------|------------------|---------------|
| `gguard-setup install` | **system** (needs admin) | **user** (no admin) |
| `start` | **system** (needs admin) | **user** |
| `stop` | **system** (needs admin) | **user** |
| `restart` | **system** (needs admin) | **user** |
| `status` | **system** (no admin needed) | **user** |
| `import` | **system** (if admin) / **user** (if not) | **user** |
| `export` | **system** (if admin) / **user** (if not) | **user** |

```
# As a regular user — operates on your user service
gguard status

# As admin — operates on system service
sudo gguard status

# Explicit user override (even if running as admin)
sudo gguard status --user
```

The `--user` flag may be placed before or after the command (`gguard --user status` or `gguard status --user`).

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
gguard import backup.ggs

# Import a guards.json (merges with existing, never loses existing keys)
gguard import config.json

# Export current scripts and config (lists paths)
gguard export
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
| `version` | Routine version (optional, defaults to `1.0`; only `1.x` is accepted) |
| `os` | Target system(s): `windows`, `linux`, `darwin`. Optional — omit to run everywhere; accepts a comma list (`os windows,linux`) |
| `watch` | Directory to monitor (absolute path or with `{home}`) |
| `debounce` | Silence time after the last event (e.g., `5s`, `10s`, `1m`) |
| `zip` | Compresses `{source}` into `{destination}.zip` |
| `sync` | Syncs `{source}` → `{destination}` (incremental) |
| `sync --delete` | Sync + removes orphans (requires `trusted`) |
| `sync --bidir` | Bidirectional sync (copies both ways) |
| `sync --bidir --delete` | Bidirectional sync with orphan removal |
| `message` | Post-execution log line **and** desktop notification |
| `notify` | Desktop notification only (no log line) |
| `run <command>` | Runs a shell command (requires `trusted: true`) |
| `delete <path>...` | Removes files/directories (requires `trusted: true`) |
| `webhook <url> [body]` | POSTs `body` to `url` (15s timeout) |

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

Use `-q` or `--quiet` to force log-to-file in foreground mode. A process started
without a console (the logon entry, the Start Menu shortcut) writes to the log
file automatically, since there is nowhere else for its output to go.

### 13. Command Line

Running `gguard` with no command prints the help. Installing and upgrading live
in `gguard-setup`, which `gguard` points you to; uninstalling is the app's own.

| Command | Description |
|---------|-------------|
| `gguard uninstall [--user]` | Remove GopherGuard, its service and its shortcuts |
| `gguard list` | List installed guards with autostart flag and watch path |
| `gguard check <file>` | Validate a `.ggs` script and print the resolved plan (dry run) |
| `gguard run <file>` | Run a `.ggs` script once, in the foreground |
| `gguard tray` | Run in the foreground with a system tray icon |
| `gguard tray --on` / `--off` | Turn the tray on / off (a persistent setting) |
| `gguard gui` | Open the web dashboard (+ the tray, if it is turned on) |
| `gguard logs [-f]` | Show the log file (`--follow` to keep it open) |

The Start Menu shortcut (Windows) and the desktop entry (Linux) open the
dashboard. Everything Windows starts by itself goes through `gguard-launcher`,
which runs `gguard` without a console; in a terminal you talk to `gguard`
directly.

| Setup command | Description |
|---------------|-------------|
| `gguard-setup` | Show the installer menu (this is what a double-click does) |
| `gguard-setup install [--user]` | Install or upgrade GopherGuard |
| `gguard-setup upgrade` | Reinstall, keeping the same install type |
| `gguard-setup uninstall [--user]` | Same as `gguard uninstall`, for when the installer is at hand |
| `gguard-setup -s <path> -g <path>` | Also install scripts and a `guards.json` |

### 14. Tray & Notifications

The tray shows the GopherGuard icon with a small status badge in the bottom-right corner, and the menu lists **every registered guard with its own colored dot**, so you can see how each one is doing.

**The tray is a setting, not a command.** `gguard tray --on` turns it on and
`gguard tray --off` turns it off; that choice is stored (a per-user logon entry
on Windows) and every entry point respects it:

| When the tray is **on** | When the tray is **off** |
|-------------------------|--------------------------|
| It starts at logon | It does not start at logon |
| `gguard gui` and the Start Menu shortcut show the tray icon | They show only the dashboard |
| The tray menu can open the dashboard | — |

A fresh install turns the tray **on**; `install`/`upgrade` over an existing
installation keeps whatever you chose. Only one GopherGuard process runs the
guards at a time, so a second `gguard gui` just reopens the dashboard that is
already running.

`gguard tray` (no flags) shows the tray right now in the foreground, without
touching the setting.

It works even with no scripts installed — in that case it simply shows the **service state**.

| Color | Meaning |
|-------|---------|
| 🟢 Green | Last run finished successfully, or the service is running (no scripts) |
| 🟡 Yellow | Idle — nothing has run yet, the service is stopped, or no guards |
| 🔴 Red | The last run failed |
| 🔵 Blue | A guard is running right now |

Clicking a guard runs that guard; **Run all now** runs every guard. The other entries open the dashboard, the log and the install folder, or quit the tray.

When a guard uses the `message` or `notify` verb, GopherGuard also raises a native desktop notification (Windows toast, `notify-send` on Linux, `osascript` on macOS).

### 15. Dashboard (Web GUI)

```bash
gguard gui
```

Serves a small dashboard on `http://127.0.0.1:8642` and opens it in your
browser, running the guards (and the tray icon, when the tray is on) in the
same process. It has no dependencies and needs no extra runtime — the page is
embedded in the binary.

| What | Detail |
|------|--------|
| **Status** | Overall state dot plus the service state (`running` / `stopped` / `not installed`) |
| **Guards** | Name, watch path, live state, run count and the last error for each guard |
| **Run all now** | Triggers every guard immediately |
| **Run** | Triggers a single guard |
| **Log** | Tail of the process log |
| **Quit** | Stops the guards and closes the tray and the dashboard |

The page refreshes itself every 2 seconds. The listener is bound to loopback
only, and requests from any other host name are rejected — so nothing outside
your machine can trigger a guard.

The same dashboard is served by the tray, which is why the tray menu has an
**Open dashboard** entry and why `gguard gui` only reopens the existing page
when the tray is already running. Started from a launcher (shortcut, logon
entry) instead of a shell, GopherGuard drops its console window and logs to
`gguard.log`, so nothing flashes on screen.

---

- [README](/README.md)
- [GG Script](/docs/GG-Script.md)
- [Guards JSON](/docs/Guards-JSON.md)