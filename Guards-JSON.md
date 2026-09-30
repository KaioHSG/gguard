# Guards JSON

The `guards.json` file provides per-routine configuration variables for `.ggs` scripts. Each top-level key corresponds to the `.ggs` file name (without extension), and its values are injected as `{key}` variables in the script.

### Config Locations

`guards.json` can exist in multiple locations depending on the installation mode:

| Location | Mode | Who manages |
|----------|------|-------------|
| `%ProgramFiles%\GopherGuard\guards.json` | **System** | Admin |
| `%APPDATA%\Programs\GopherGuard\guards.json` | **User** | Current user |
| `~/.local/bin/guards.json` (Linux user) | **User** | Current user |
| `/usr/local/bin/guards.json` (Linux system) | **System** | Admin/root |

When the **system service** runs, it loads **both** the global config and the active user's config. Each script uses the config from its own directory (global scripts use global config, user scripts use user config).

### Example

```json
{
  "backup": {
    "dest": "${HOME}/OneDrive/Backups",
    "trusted": true,
    "autostart": true,
    "free_cache": "1h",
    "keep_backups": 5,
    "delete_older_than": "30d"
  }
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `dest` | string | Backup/sync destination path. Becomes `{dest}` in the script |
| `trusted` | bool | Allows sensitive commands (e.g., delete old backups) |
| `autostart` | bool | Automatically starts when the service is running |
| `free_cache` | string | Delay to release local OneDrive cache (e.g., `"1h"`, `"30m"`) |
| `keep_backups` | int | Maximum `.zip` files kept in the destination (requires `trusted: true`) |
| `delete_older_than` | string | Removes zips older than the period (e.g., `"7d"`, `"24h"`) |

- `${HOME}` is resolved to the current user's home directory at load time.
- `os.ExpandEnv` is applied, so `%APPDATA%`, `$HOME`, etc. work.

### Import & Merge

Use `gguard --import config.json` to merge an external config. The merge is **key-replace**:

- Keys in the imported file **create or fully overwrite** matching keys in the existing file
- Keys NOT present in the import are **preserved**
- Never replaces the entire file

Example:
```json
// Existing:
{ "backup": { "dest": "D:/old", "keep_backups": 3, "trusted": true },
  "fotos":  { "dest": "D:/fotos" } }

// Imported:
{ "backup": { "dest": "D:/joao" } }

// Result:
{ "backup": { "dest": "D:/joao" },  ← replaced entirely
  "fotos":  { "dest": "D:/fotos" } } ← preserved
```