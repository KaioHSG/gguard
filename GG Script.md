# GG Script

GG Script é a DSL (Domain-Specific Language) do GopherGuard para definir rotinas de monitoramento e backup.

### Exemplo:

`backup.ggs`

```nidx
# Rotina de backup

guard "Meu Backup"
version 1.0
os windows

watch "{home}/Documents/Projetos"
debounce 5s
zip {backup}/projetos-{timestamp}.zip {watch}
message "Backup salvo em \"{backup}\""
```

### Linha a linha:

| Linha	| O que é |
|-------|---------|
| `guard "Meu Backup"` | Cabeçalho obrigatório. Nome da rotina |
| `version 1.0` | Cabeçalho obrigatório. Versão da rotina |
| `os windows` | Cabeçalho obrigatório. Sistema alvo |
| `watch "..."` | Registra um watcher recursivo nesse diretório |
| `debounce 5s` | Tempo de silêncio após o último evento antes de disparar |
| `zip <destino.zip> <origem>` | Comando de compactação ZIP |
| `sync <destino> <origem>` | Sincronização incremental (copia só arquivos novos/alterados) |
| `sync <destino> <origem> --delete` | Sync com remoção de órfãos (requer `trusted: true`) |
| `sync <destino> <origem> --bidir` | Sync bidirecional (copia nos dois sentidos) |
| `sync <destino> <origem> --bidir --delete` | Sync bidirecional com remoção de órfãos |
| `message "..."` | Log/notificação pós-execução |

### Variáveis disponíveis no contexto:

| Variável | Resolve para | De onde vem |
|----------|-------------|-------------|
| `{watch}` | O path do watch ativo | engine injeta na hora do disparo |
| `{dest}` | Pasta destino | config global do gguard (`guards.json`) |
| `{timestamp}` | Ex: `2026-09-23_14-30-05` | engine gera no momento do disparo |
| `{file}` | Nome do arquivo que acionou o evento | engine injeta (opcional, se houver 1 arquivo) |
| `{nome}` | Qualquer chave extra do `guards.json` | config global |

### Configurações do guards.json

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `dest` | string | Pasta destino para backup/sync |
| `trusted` | bool | Permite comandos sensíveis (ex: deletar backups antigos) |
| `autostart` | bool | Inicia automaticamente ao abrir o gg-launcher |
| `free_cache` | string | Delay para liberar cache do OneDrive (ex: `"1h"`, `"30m"`) |
| `keep_backups` | int | Máximo de zips mantidos (requer `trusted: true`) |
| `delete_older_than` | string | Deleta zips mais velhos que (ex: `"7d"`, `"24h"`) |