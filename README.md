# GopherGuard (gguard)

*Serviço leve, executável e multiplataforma escrito em Go para monitorar alterações em diretórios e disparar tarefas customizadas de forma assíncrona — backup automático, sincronização, notificações, etc.*

### 1. Arquitetura

Três módulos principais, com baixo acoplamento:

- **Filesystem Watcher (fsnotify)**: Escuta eventos do sistema operacional (criação, modificação, exclusão) nos caminhos especificados.
- **Debouncer / Timer Engine**: Filtro inteligente. Aplicativos frequentemente salvam arquivos em rajadas (vários eventos por segundo). O engine aguarda um período de "silêncio" (ex: 5 segundos sem alterações) antes de liberar a execução da tarefa.
- **Task Dispatcher**: Executor. Uma fila de tarefas customizáveis que recebe o gatilho validado e executa a rotina definida (ex: compactar em .zip, enviar para OneDrive).

### 2. Stack Tecnológica (Go)

- **Go**: Binário único e estático, baixo consumo de RAM, excelente suporte a concorrência (goroutines).
- **fsnotify**: Biblioteca padrão para monitoramento de eventos de arquivos de forma multiplataforma (ReadDirectoryChangesW no Windows, inotify no Linux).
- **Configuração**: Scripts `.ggs` (DSL declarativa) + `guards.json` (configuração privada por rotina).

### 3. Fluxo de Execução

1. **Inicialização**: O binário lê o script `.ggs` e o `guards.json` com as regras de monitoramento.
2. **Registro de Watchers**: Para o diretório alvo, o Go inicia uma goroutine usando o fsnotify (suporte a subdiretórios recursivo).
3. **Captura de Eventos**: O sistema de arquivos dispara eventos de modificação (WRITE / CREATE).
4. **Debounce**: O evento cancela o timer anterior e reinicia a contagem. Se novos eventos chegarem, o timer é resetado. Quando o timer zera (fim da rajada de alterações), o evento consolidado é enviado para a fila de tarefas.
5. **Disparo da Tarefa**: O Dispatcher executa as ações configuradas (ex: zip, mensagem de log) com as variáveis resolvidas.

### 4. Exemplo de Script (.ggs)

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

### 5. Uso

```
gguard -script example\backup.ggs -config example\guards.json
```

### 6. Variáveis de Contexto

| Variável | Resolve para |
|----------|-------------|
| `{home}` | Diretório home do usuário |
| `{watch}` | Caminho monitorado |
| `{dest}` | Destino do backup/sync (de `guards.json`) |
| `{timestamp}` | Data/hora atual (`2026-09-23_12-30-00`) |
| `{file}` | Nome do arquivo que disparou o evento |

### 7. Comandos Disponíveis

| Comando | Descrição |
|---------|-----------|
| `guard` | Nome da rotina (obrigatório) |
| `version` | Versão da rotina (obrigatório) |
| `os` | Sistema alvo: `windows`, `linux`, `darwin` |
| `watch` | Diretório a ser monitorado (caminho absoluto ou com `{home}`) |
| `debounce` | Tempo de silêncio após o último evento (ex: `5s`, `10s`, `1m`) |
| `zip` | Compacta `{origem}` em `{destino}.zip` |
| `sync` | Sincroniza `{origem}` → `{destino}` (incremental) |
| `sync --delete` | Sync + remove órfãos (requer `trusted`) |
| `sync --bidir` | Sync bidirecional (copía nos dois sentidos) |
| `sync --bidir --delete` | Sync bidirecional com remoção de órfãos |
| `message` | Log / notificação pós-execução |

### 8. Configurações do guards.json

| Campo | Descrição |
|-------|-----------|
| `dest` | Pasta destino |
| `trusted` | Permite comandos sensíveis (limpeza de backups) |
| `autostart` | Inicia com `gg-launcher.exe` |
| `free_cache` | Libera cache OneDrive após (ex: `"1h"`) |
| `keep_backups` | Máximo de zips mantidos (requer `trusted`) |
| `delete_older_than` | Remove zips mais velhos (ex: `"7d"`) |