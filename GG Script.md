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
| `version 1.0` | Required header. Routine version |
| `os windows` | Required header. Target system |
| `watch "..."` | Registers a recursive watcher on this directory |
| `debounce 5s` | Silence time after the last event before triggering |
| `zip <destination.zip> <source>` | ZIP compression command |
| `sync <destination> <source>` | Incremental sync (copies only new/changed files) |
| `sync <destination> <source> --delete` | Sync with orphan removal (requires `trusted: true`) |
| `sync <destination> <source> --bidir` | Bidirectional sync (copies both ways) |
| `sync <destination> <source> --bidir --delete` | Bidirectional sync with orphan removal |
| `message "..."` | Post-execution log/notification |

### Available context variables:

| Variable | Resolves to | Source |
|----------|-------------|--------|
| `{watch}` | The active watch path | engine injects at dispatch time |
| `{dest}` | Destination folder | gguard global config (`guards.json`) |
| `{timestamp}` | E.g., `2026-09-23_14-30-05` | engine generates at dispatch time |
| `{file}` | Name of the file that triggered the event | engine injects (optional, if there is 1 file) |
| `{name}` | Any extra key from `guards.json` | global config |

### guards.json Settings

| Field | Type | Description |
|-------|------|-------------|
| `dest` | string | Destination folder for backup/sync |
| `trusted` | bool | Allows sensitive commands (e.g., delete old backups) |
| `autostart` | bool | Automatically starts when gg-launcher opens |
| `free_cache` | string | Delay to release OneDrive cache (e.g., `"1h"`, `"30m"`) |
| `keep_backups` | int | Maximum zips kept (requires `trusted: true`) |
| `delete_older_than` | string | Deletes zips older than (e.g., `"7d"`, `"24h"`) |