# Guards JSON

`guards.json` file — global configuration for all routines. Each top-level key corresponds to the `.ggs` file name (without extension), and its values are injected as `{key}` variables in the script.

```json
{
  "my-backup": {
    "dest": "${HOME}/OneDrive/Backups",
    "trusted": true,
    "autostart": true,
    "free_cache": "1h",
    "keep_backups": 5,
    "delete_older_than": "30d"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `dest` | string | Backup/sync destination path. Becomes `{dest}` in the script |
| `trusted` | bool | Allows sensitive commands (e.g., delete old backups) |
| `autostart` | bool | Automatically starts when `gg-launcher.exe` opens |
| `free_cache` | string | Delay to release local OneDrive cache (e.g., `"1h"`, `"30m"`) |
| `keep_backups` | int | Maximum `.zip` files kept in the destination (requires `trusted: true`) |
| `delete_older_than` | string | Removes zips older than the period (e.g., `"7d"`, `"24h"`) |

- `${HOME}` is resolved by the engine to the current user's home directory.