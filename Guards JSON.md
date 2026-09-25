# Guards JSON

Arquivo `guards.json` — configuração global de todas as rotinas. Cada chave de nível 1 corresponde ao nome do arquivo `.ggs` (sem extensão), e seus valores são injetados como variáveis `{chave}` no script.

```json
{
  "meu-backup": {
    "dest": "${HOME}/OneDrive/Backups",
    "trusted": true,
    "autostart": true,
    "free_cache": "1h",
    "keep_backups": 5,
    "delete_older_than": "30d"
  }
}
```

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `dest` | string | Caminho destino do backup/sync. Vira `{dest}` no script |
| `trusted` | bool | Permite comandos sensíveis (ex: deletar backups antigos) |
| `autostart` | bool | Inicia automaticamente ao abrir `gg-launcher.exe` |
| `free_cache` | string | Delay para liberar cache local do OneDrive (ex: `"1h"`, `"30m"`) |
| `keep_backups` | int | Máximo de arquivos `.zip` mantidos no destino (requer `trusted: true`) |
| `delete_older_than` | string | Remove zips mais antigos que o período (ex: `"7d"`, `"24h"`) |

- `${HOME}` é resolvido pelo engine como a home do usuário atual.