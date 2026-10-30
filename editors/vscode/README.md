# GopherGuard for VS Code

Syntax highlighting for the two files GopherGuard reads:

| File | Language | What it highlights |
|------|----------|--------------------|
| `*.ggs` | `ggs` | Verbs, `--flags`, strings, `{variables}`, durations, `# comments`, the guard name |
| `.ggignore` | `ggignore` | `# comments`, `!` negation, `*`/`**`/`?`, `/` anchors, paths |

It is a plain **TextMate grammar**: no build step, no language server, nothing
running in the background. The scopes are the standard ones, so whatever theme
you use decides the colors.

## Trying it

From this directory (`editors/vscode`):

**The quick way** — copy the folder into your extensions directory and restart
VS Code. The folder name has to follow `<publisher>.<name>-<version>`, and the
manifest needs a `publisher`, or VS Code silently ignores it:

```powershell
Copy-Item -Recurse . "$env:USERPROFILE\.vscode\extensions\kaiohsg.gguard-0.1.3"
code --list-extensions | Select-String gguard   # must print kaiohsg.gguard
```

**The tidy way** — package it and install the `.vsix` (it then shows up in the
Extensions list, and updates like any other):

```bash
npx @vscode/vsce package
code --install-extension gguard-0.1.0.vsix
```

**In one profile** — with [profiles][profiles], only the *Default* profile reads
the extensions directory. Every other profile keeps its own list, so the folder
copy above is invisible there; install the `.vsix` with `--profile`:

```bash
code --profile "Go" --install-extension gguard-0.1.0.vsix
code --profile "Go" --list-extensions | Select-String gguard   # kaiohsg.gguard
```

[profiles]: https://code.visualstudio.com/docs/configure/profiles

**While developing it** — open this folder in VS Code and press `F5`: a new
window opens with the extension loaded.

The packaged `gguard-<version>.vsix` is a build artifact: it is ignored by git
and can be deleted after installing.

## Checking it works

Open `example/backup.ggs` from the repository root: `guard`, `watch`,
`debounce`, `zip` and `message` should be keywords, `"My Backup"` and the paths
strings, `{home}` / `{watch}` / `{dest}` / `{timestamp}` variables, `5s` and
`1.0` numbers, and the `# Backup routine` line a comment.

For the ignore file, any `.ggignore` should show comments and patterns — in
this repository, the routines' own `sync --exclude` rules are documented in
[GG Script](../../docs/GG-Script.md).

## Publishing (not done yet)

The extension is kept in the repository without being distributed. The
`publisher` in `package.json` (`kaiohsg`) is there so VS Code accepts the
manifest; publishing for real also needs that publisher registered on the
Marketplace (`vsce login`, `vsce publish`), and ideally a `license` and an
icon.
