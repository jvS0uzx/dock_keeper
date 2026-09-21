# Arquitetura

O painel tem **quatro fontes de dado independentes**. Elas não se substituem: cada
uma enxerga o que as outras não alcançam, e cada uma tem um ponto cego que é
preciso conhecer antes de escolher.

```
   coleta SSH          agente push        coletor remoto      varredura local
  (host servidor)   (estação/servidor)    (rede da filial)    (rede do painel)
        │                   │                    │                    │
        │ stream contínuo   │ POST periódico     │ POST periódico     │ ciclo interno
        ▼                   ▼                    ▼                    ▼
  ssh.Manager        /api/ingest/metrics  /api/ingest/inventory  discovery.Sweeper
        │                   │                    │                    │
        └───────────────────┴────────┬───────────┴────────────────────┘
                                     ▼
                                 PostgreSQL
                                     │
                    ┌────────────────┼────────────────┐
                    ▼                ▼                ▼
              motor de regras   worker de SSL    API HTTP + SSE
                    │                │                │
                    └────► Telegram ◄┘                ▼
                                                   Frontend
```

## 1. Coleta por SSH

**O que é.** Para cada servidor cadastrado com `Kind = "ssh"`, o painel abre uma
sessão SSH persistente no boot (`startCollectors` em `cmd/dockkeeper/main.go`) e
envia um script pelo stdin de um `bash -s`. O script fica num laço, imprimindo
uma linha JSON a cada `SSH_COLLECT_INTERVAL` segundos, e o painel a consome sem
reconectar.

O script está embutido no binário (`scripts/stream_metrics.sh`, via `go:embed`).
Antes ele era lido do disco em runtime, o que amarrava o serviço ao diretório de
trabalho e quebrava a coleta quando o deploy copiava só o executável.

**O que ela dá.** CPU, memória, disco, load, uptime, temperatura, a lista de
containers com estado e consumo, o access log do Nginx (quando `collect_nginx`
está ligado no servidor) e o `auth.log`. É a única fonte que permite **agir** no
host: `start`, `stop` e `restart` de container passam por aqui.

**O que ela não faz.** Não alcança máquina sem SSH — estação Windows, dispositivo
de rede, impressora. Não funciona sem chave: `SSH_KEY_PATH` é uma chave só para
toda a frota, em geral `root`, e essa é uma dívida de segurança conhecida.

**Ponto cego.** O intervalo é o do script remoto, não do painel. Um host cujo
script morreu continua com a última métrica no banco até a janela de liveness
expirar — ver [`metricas.md`](metricas.md).

## 2. Agente de push

**O que é.** Um binário Go (`cmd/agent`) instalado na máquina monitorada, que
mede localmente e faz `POST /api/ingest/metrics` a cada `AGENT_INTERVAL`
segundos. Cross-compilado para Linux (amd64/arm64), Windows e macOS pelo
`backend/Makefile`.

**O que ele dá.** As mesmas métricas de host da coleta SSH, mais o usuário
logado, o sistema operacional e a versão do agente. Funciona atrás de NAT e sem
porta aberta: quem inicia a conexão é a máquina monitorada.

**O que ele não faz.** Não lê containers, não abre stream de log, e **não aceita
comando**. `startCollectors` pula explicitamente hosts com `Kind = "agent"` —
tentar SSH neles geraria alerta de host inalcançável em laço.

**Identidade.** Cada agente tem credencial própria, amarrada a uma unidade, obtida
por um convite de uso único. O token compartilhado antigo continua aceito durante
a transição, com aviso no log. Ver [`agente.md`](agente.md).

## 3. Coletor remoto de inventário

**O que é.** O repositório `dockkeeper_collector`, instalado numa máquina dentro da rede
da filial. Varre a faixa local e faz `POST /api/ingest/inventory`.

**Por que existe.** O painel só enxerga a rede onde ele mesmo roda. Uma empresa
com dez filiais precisaria de dez painéis, ou de um coletor por filial.

**O que ele dá.** Hosts descobertos com IP, hostname, MAC, portas abertas e o tipo
inferido do equipamento.

**O que ele não faz.** Não coleta métrica, não conhece Docker, não abre sessão
em ninguém. É inventário e só.

## 4. Varredura local

**O que é.** Uma goroutine dentro do próprio painel (`internal/discovery`), que
varre as faixas de `DISCOVERY_CIDRS` a cada `DISCOVERY_INTERVAL_MIN` minutos.

**O que ela dá.** O mesmo que o coletor remoto, para a rede onde o painel roda —
literalmente o mesmo código: desde 19/09/2026 o painel importa o pacote
`github.com/jvS0uzx/dockkeeper_collector/scan`, em vez de manter uma cópia que já
tinha divergido. `internal/discovery` ficou só com o que é do painel: a
classificação por porta e a persistência em `network_hosts`.

**O que ela não faz.** Só aceita faixa privada (RFC1918) e recusa prefixo maior
que `/16` — 65 mil hosts é varredura longa demais para o caso de uso e
provavelmente erro de digitação.

**Ponto cego importante.** A resolução de MAC lê `/proc/net/arp`. Dentro de um
container isso é a tabela ARP do *namespace*, não a do host: a varredura enxerga
quase nada. Ver [`dependencias.md`](dependencias.md).

**Ela se desliga sozinha** quando a unidade já tem um coletor remoto registrado.
Dois escritores no mesmo inventário se sobrescrevem a cada ciclo, e a diferença
entre as listas de porta faz o tipo do equipamento alternar sozinho. Ver
[`inventario-de-rede.md`](inventario-de-rede.md).

---

## O que roda em segundo plano

| Rotina | Intervalo | O que faz |
|---|---|---|
| `StartTrendWorker` | 15 min | Agrega a métrica bruta em médias horárias |
| `StartRetentionWorker` | 1 h | Poda métricas e auditoria vencidas, em lotes |
| `logstore.StartRetention` | 1 h | Poda linhas de log vencidas |
| `network.StartSSLWorker` | 30 min | Revalida certificados e alerta os que expiram |
| `rules.StartEngine` | 30 s | Avalia as regras de alerta sobre a última métrica |
| `discovery.Sweeper.Start` | `DISCOVERY_INTERVAL_MIN` | Varre as faixas configuradas |

A ordem entre o rollup e a poda não é acidental: `StartTrendWorker` devolve um
canal que a poda espera antes da primeira passada. Se o painel ficou fora do ar
mais tempo que a janela de retenção, podar antes de agregar apagaria dado bruto
que o rollup nunca consolidou — e o histórico daquele período sumiria para
sempre.

## Transporte para o navegador

**SSE, não WebSocket.** Não há WebSocket em lugar nenhum do código. As telas de
tempo real usam `text/event-stream`, que é unidirecional e basta: o navegador
não precisa mandar nada de volta por esse canal.

As rotas SSE são autorizadas por **ticket de uso único** (`POST
/api/stream-ticket`), e não pelo token permanente. O motivo é que `EventSource`
não deixa definir cabeçalho: a credencial teria que ir na query string, onde
acabaria no access log do proxy e no histórico do navegador.

Atrás de proxy reverso, o nginx precisa de `proxy_buffering off` — senão ele
segura os eventos e a tela fica parada, sem erro nenhum aparecer. Ver
[`backend/deploy/README.md`](../backend/deploy/README.md).

## Banco

PostgreSQL, com GORM para consulta. O esquema sobe **só** pelas migrações versionadas em
`internal/database/migracoes/` (ADR 012); o `AutoMigrate` e `DB_AUTOMIGRATE` foram removidos,
porque criavam um banco sem chaves estrangeiras, CHECKs e índices. Os índices que tag de GORM
não expressa (o de expressão do inventário e o parcial de `machine_id`) vivem no baseline.

O pool é configurado explicitamente (`DB_MAX_OPEN_CONNS` e companhia). Sem teto,
as goroutines de coleta somadas aos handlers HTTP esgotam o `max_connections` do
Postgres com `FATAL: sorry, too many clients`.

## Registro único de métrica

Tudo o que o backend sabe sobre uma métrica mora em `internal/metricas`: nome, rótulo, unidade,
escopo (servidor, container ou ambos), a expressão SQL na série bruta, as colunas de média e
máxima na tendência (ou a ausência delas) e o acessor que o motor de regras usa. O histórico, a
validação de regra e de painel, o motor e o `GET /api/metrics/catalogo` consultam esse registro;
não existe outra lista de métricas no backend, e o frontend lê o catálogo.

Acrescentar uma métrica toca **cinco** arquivos:

1. `internal/metricas/metricas.go`, a entrada no registro;
2. `internal/database/schema.go`, os campos em `MetricServer` e, se houver tendência, em `MetricServerTrend`;
3. uma migração nova em `internal/database/migracoes/`, com as colunas e, se o motor avalia a
   métrica, o `ck_alert_rules_metric` recriado (o CHECK é SQL e não lê Go; o teste
   `TestCheckDoBancoBateComORegistro` falha enquanto os dois divergirem);
4. `internal/database/trends.go`, a agregação da tendência, quando houver;
5. o ponto de coleta: `internal/ssh/client.go` para SSH ou `internal/api/ingest.go` para o agente.
   Métrica coletada pelos dois caminhos toca os dois, e aí são seis.

## Dívida registrada: estado global do pacote `alert`

`internal/alert` guarda em variáveis de pacote o canal (`token`, `chatID`, `enabled`, `apiBase`), o
`cooldown`, o relógio `agora`, o mapa `lastSent` e a saúde do canal. Os testes do pacote trocam
esses valores direto e restauram no `Cleanup`; são cerca de cinquenta pontos em sete arquivos, e é
por isso que o pacote só roda com `-p 1` e não aceita `t.Parallel()`.

O conserto é um tipo `Canal` com esses campos, criado no `main` e injetado em quem alerta. Não
foi feito na Missão 5: reescreveria todo o conjunto de testes do pacote mais crítico do painel sem
mudar comportamento, e o risco não se pagava no mesmo ciclo em que o ciclo de vida do alerta
mudou. Já está isolado o que custava pouco: os gatilhos de `ssh` e `network` recebem `notifyAlert`
e `resolveAlert` injetáveis, e nenhum teste fora do pacote `alert` toca o estado dele. Quem pegar:
comece por `telegram.go` e `saude.go`, que concentram as variáveis, e troque `setupFila` primeiro.
