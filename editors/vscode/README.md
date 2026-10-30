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
Copy-Item -Recurse . "$env:USERPROFILE\.vscode\extensions\kaioshg.gguard-0.1.0"
code --list-extensions | Select-String gguard   # must print kaioshg.gguard
```

**The tidy way** — package it and install the `.vsix` (it then shows up in the
Extensions list, and updates like any other):

```bash
npx @vscode/vsce package
code --install-extension kaioshg.gguard-0.1.0.vsix
```

**While developing it** — open this folder in VS Code and press `F5`: a new
window opens with the extension loaded.

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
`publisher` in `package.json` (`kaioshg`) is there so VS Code accepts the
manifest; publishing for real also needs that publisher registered on the
Marketplace (`vsce login`, `vsce publish`), and ideally a `license` and an
icon.
