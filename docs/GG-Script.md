# GG Script

GG Script is GopherGuard's DSL (Domain-Specific Language) for defining monitoring and backup routines.

### Example:

`backup.ggs`

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

### Line by line:

| Line | What it is |
|------|------------|
| `guard "My Backup"` | Required header. Routine name |
| `version 1.0` | Optional header. Routine version (defaults to `1.0`; only `1.x` is accepted) |
| `os windows` | Optional header. Target system(s): `windows`, `linux`, `darwin`. Omit to run everywhere; accepts a comma list (`os windows,linux`) |
| `watch "..."` | Registers a recursive watcher on this directory |
| `debounce 5s` | Silence time after the last event before triggering |
| `zip <destination.zip> <source>` | ZIP compression command |
| `sync <destination> <source>` | Incremental sync (copies only new/changed files) |
| `sync <destination> <source> --delete` | Sync with orphan removal (requires `trusted: true`) |
| `sync <destination> <source> --bidir` | Bidirectional sync (copies both ways) |
| `sync <destination> <source> --bidir --delete` | Bidirectional sync with orphan removal |
| `sync <destination> <source> --exclude <pattern>` | Leaves matching paths behind (repeatable) |

#### Leaving things behind

`--exclude` can be repeated, and a `.ggignore` file at the **root of the source**
is read as well — one pattern per line, `#` starts a comment:

```nidx
sync {dest} {watch} --exclude dist --exclude .git --exclude *.exe
```

```text
# .ggignore, next to what is being watched
dist/
.git/
*.exe
!portable.exe      # an exception, since later patterns win
logs/**/tmp
```

The syntax is the one people already know from git's ignore files:

| Pattern | Matches |
|---------|---------|
| `dist` | a file or directory named `dist`, at any depth |
| `dist/` | a directory named `dist` (and everything inside it) |
| `/dist` | only the `dist` at the root of the source |
| `*.exe` | any name ending in `.exe`, at any depth |
| `cmd/**/tmp` | `cmd/tmp`, `cmd/a/tmp`, `cmd/a/b/tmp`… |
| `!keep.exe` | re-includes what an earlier pattern excluded |

A pattern that matches a directory drops the whole tree, so there is no point in
re-including something whose parent is excluded (the same rule git documents).
Excluded paths are never copied, and `--delete` never removes them from the
destination.
| `message "..."` | Post-execution log line **and** desktop notification |
| `notify "..."` | Desktop notification only |
| `run <command>` | Runs a shell command (requires `trusted: true`) |
| `delete <path>...` | Removes files/directories (requires `trusted: true`) |
| `webhook <url> [body]` | POSTs `body` to `url` |

### Available context variables:

| Variable | Resolves to | Source |
|----------|-------------|--------|
| `{watch}` | The active watch path | engine injects at dispatch time |
| `{dest}` | Destination folder | gguard global config (`guards.json`) |
| `{timestamp}` | E.g., `2026-09-23_14-30-05` | engine generates at dispatch time |
| `{file}` | Name of the file that triggered the event | engine injects (optional, if there is 1 file) |
| `{name}` | Any extra key from `guards.json` | global config |

### Script Locations

Scripts are loaded from different directories depending on the installation mode:

| Installation | Script directory |
|-------------|------------------|
| **System** (admin) | `%ProgramFiles%\GopherGuard\gg-scripts\` (Windows) |
| **User** (no admin) | `%APPDATA%\Programs\GopherGuard\gg-scripts\` (Windows) |
| **System** (sudo) | `/usr/local/bin/gg-scripts/` (Linux) |
| **User** (no sudo) | `~/.local/bin/gg-scripts/` (Linux) |

When the **system service** runs, it also loads scripts from the **active user's** config directory (the person currently logged in at the console). This allows each user to have personal backup scripts without needing admin privileges.

To import a script: `gguard import backup.ggs` (copies to the correct directory automatically).

To validate a script before installing it: `gguard check backup.ggs` (dry run). To run it once right now: `gguard run backup.ggs`.

### guards.json Settings

| Field | Type | Description |
|-------|------|-------------|
| `dest` | string | Destination folder for backup/sync |
| `trusted` | bool | Allows sensitive commands (e.g., delete old backups) |
| `autostart` | bool | Automatically starts when the service is running |
| `free_cache` | string | Delay to release OneDrive cache (e.g., `"1h"`, `"30m"`) |
| `keep_backups` | int | Maximum zips kept (requires `trusted: true`) |
| `delete_older_than` | string | Deletes zips older than (e.g., `"7d"`, `"24h"`) |

---

- [README](/README.md)
- [GG Script](/docs/GG-Script.md)
- [Guards JSON](/docs/Guards-JSON.md)