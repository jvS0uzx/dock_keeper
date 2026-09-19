# API

Extraído de `internal/api/server.go`, que é a lista autoritativa. São **32
rotas**.

## Como ler a coluna "exige"

| Gate | Significado |
|---|---|
| `viewer` | Qualquer papel autenticado |
| `viewer` / `operator` | Leitura para todos, escrita a partir de operador |
| `viewer` / **operador global** | Leitura livre, escrita exige o papel em concessão global |
| **admin global** | `admin` com concessão global, **inclusive na leitura** |
| ticket | Ticket de uso único de `POST /api/stream-ticket` |
| público | Sem credencial |
| dispositivo | Credencial de dispositivo (`X-Device-Id` + `X-Device-Token`). O `AGENT_INGEST_TOKEN` legado só vale com `ALLOW_LEGACY_INGEST_TOKEN=true` |

"Global" quer dizer concessão sem unidade. Ver
[`autenticacao.md`](autenticacao.md).

---

## Métricas

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/metrics/live` | GET | `viewer` | Último estado conhecido de hosts, containers e balanceador. É o que o painel consulta em polling. Por servidor traz `cpu`, `load1`, `temperature_c`, `net_rx_bps`, `net_tx_bps` e `rtt_ms`, `null` quando não há medição |
| `/api/metrics/history` | GET | `viewer` | Série temporal de uma métrica, com janela. Lê a tendência agregada nas janelas longas. `metric`: `cpu`, `mem`, `disk`, `load`, `temperature`, `latency` (handshake SSH), `rtt` (ms), `net_rx` e `net_tx` (bytes/s) |
| `/api/logs/search` | GET | `viewer` | Busca no histórico de linhas de log, recortada por unidade |

### Janela do histórico

`/api/metrics/history` aceita **uma** das duas formas de janela:

- `range`: `1h`, `6h`, `24h`, `7d`, `30d` ou `90d`. Sem nada, vale `1h`.
- `from` e `to`, em RFC3339. `to` é opcional e vale agora.

Mandar `range` e `from` juntos, `to` sem `from`, data fora do RFC3339, `from`
igual ou posterior a `to` ou período acima de 400 dias responde 400.

Janela de até 24 h lê a série bruta, com pontos por minuto (até 1 h) ou por
5 minutos. Acima de 24 h lê a tendência horária: um ponto por hora até 30 dias,
um a cada 6 horas até 90 dias e um por dia acima disso. O período customizado segue
a mesma regra pelo seu tamanho. Container não tem tendência e sempre lê a série
bruta, que guarda 7 dias. O formato da resposta não muda: `[{"ts", "value"}]`.

## Dashboards e anotações

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/dashboards` | GET, POST, PUT, DELETE | sessão de usuário | Dashboards do próprio usuário |
| `/api/annotations` | GET, POST, DELETE | `viewer` na leitura, sessão de usuário na escrita | Marcações de evento nos gráficos |

O token de máquina recebe 403 em `/api/dashboards` e na escrita de
`/api/annotations` (`POST` e `DELETE`): um dashboard tem dono e uma anotação tem
autor. A leitura de anotações funciona com o token de máquina como qualquer outra
leitura dessa sessão: concessão global, todas as anotações do filtro. Ver o ADR
[010](adr/010-dashboards-relacionais-com-dono.md).

**Dashboards.** `GET` devolve só os do usuário da sessão:
`[{id, name, panels: [{id, position, title, server_id, metric, range, width}], updated_at}]`.
Painel cujo servidor saiu do alcance do usuário, ou foi apagado, sai da resposta
sem erro.

`POST` recebe `{name, panels: [{title, server_id, metric, range, width}]}` e
responde 201 com o objeto. `PUT ?id=N` troca o nome e todos os painéis numa
transação e responde 200. `DELETE ?id=N` apaga o dashboard e, por chave
estrangeira em cascata, os painéis. A ordem dos painéis é a do array enviado.

| Regra | Resposta |
|---|---|
| `name` vazio ou com mais de 80 caracteres | 400 |
| Mais de 12 painéis, ou 21º dashboard do mesmo usuário | 400 |
| `metric` fora de `cpu`, `mem`, `disk`, `load`, `temperature`, `net_rx`, `net_tx`, `rtt` | 400 |
| `range` fora de `1h`, `6h`, `24h`, `7d`, `30d`, `90d` | 400 |
| `width` diferente de 1 ou 2 (ausente vale 1), `title` com mais de 80, painel sem `server_id` | 400 |
| Servidor inexistente ou fora do alcance do usuário | 404 |
| Nome repetido entre os dashboards do mesmo usuário | 409 |
| Dashboard de outro usuário, em `PUT` ou `DELETE` | 404 |

**Anotações.** `GET ?server_id=&from=&to=` (RFC3339; padrão: últimas 24 h)
devolve `[{id, server_id, at, text, author, created_at}]`, ordenado por `at`.
Com `server_id`, vêm as anotações daquele servidor e as globais; sem ele, todas as
que o usuário alcança. A anotação global (`server_id: null`) aparece para todos; a
de servidor segue o recorte por unidade.

`POST` recebe `{server_id | null, at?, text}`. `at` ausente vale agora, `text` vai
de 1 a 280 caracteres. Anotar num servidor exige papel de operador na unidade
dele; anotação global exige operador com concessão global. `DELETE ?id=N` é do
autor ou de um administrador global; os outros que enxergam a anotação recebem
403, e quem não enxerga recebe 404.

Apagar um usuário apaga também os dashboards dele. As anotações dele ficam, com
`author` vazio.

As escritas das duas rotas são auditadas pelo middleware: `dashboards.create`,
`dashboards.update`, `dashboards.delete`, `annotations.create` e
`annotations.delete`.

## Servidores e containers

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/servers` | GET, POST, DELETE | **admin global** | Cadastro dos hosts monitorados. Cadastrar entrega acesso SSH root, por isso é admin |
| `/api/containers/action` | POST | **operador global** | `start`, `stop` ou `restart` de container no host remoto |
| `/api/containers/logs/stream` | GET | ticket | `docker logs -f` por SSE |

`/api/containers/action` é a única rota que audita a si própria, gravando
**antes** de o comando sair. Ver [ADR sobre a auditoria de comando](adr/).

## Segurança

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/security/radar` | GET | `viewer` | Portas em LISTEN do host (`ss -tulnp`) |
| `/api/security/authlog/stream` | GET | ticket | Log de autenticação do host por SSE |

A abertura dos dois streams é auditada — uma linha por abertura, nunca por
evento. Ler o `auth.log` de uma máquina de produção como root é leitura de dado
sensível, e "quem leu o log de autenticação do servidor X" é a pergunta que a
auditoria existe para responder.

As quatro rotas que abrem sessão SSH sob demanda (`/api/security/radar`,
`/api/security/authlog/stream`, `/api/containers/logs/stream` e
`/api/containers/action`) dividem um teto por servidor, `SSH_MAX_SESSIONS_PER_HOST`
(padrão 6). A sessão excedente recebe 503 com a mensagem "limite de sessões SSH
simultâneas atingido para este servidor", antes de abrir conexão e, nos streams,
antes de iniciar o SSE.

## SSL

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/ssl/domains` | GET, POST, DELETE | `viewer` / **operador global** | Cadastro dos domínios verificados |
| `/api/ssl/discover` | GET | `viewer` | Domínios observados no access log do Nginx que ainda não são monitorados |
| `/api/ssl/import` | POST | **operador global** | Importa um domínio descoberto para o cadastro |
| `/api/ssl/recheck` | POST | **operador global** | Revalida um domínio agora |
| `/api/ssl/recheck-all` | POST | **operador global** | Revalida todos |

## Rede e unidades

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/network/hosts` | GET | `viewer` | Inventário, recortado por unidade. Aceita `?site_id=` |
| `/api/network/scan` | POST | **operador global** | Dispara uma varredura fora do ciclo |
| `/api/network/host` | PATCH | `viewer` + papel na unidade | Cadastro do host (sala, dono, patrimônio, unidade, tipo). O papel na unidade do host é conferido **dentro** do handler |
| `/api/sites` | GET, POST, DELETE | `viewer` / `operator` | Unidades (filiais). O `DELETE` responde **409** enquanto houver credencial de dispositivo ativa ou convite válido na unidade: revogue antes, senão o dispositivo continuaria enviando para uma unidade que não existe. Sem dispositivo preso, a remoção roda numa transação única que anula `servers.site_id` e `alert_rules.target_site_id`, limpa `network_hosts` e `floor_plans` e apaga acessos, credenciais revogadas e convites gastos |

## Plantas baixas

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/floorplans` | GET, POST | `viewer` / `operator` | Lista e envia planta. Teto de corpo próprio: o corpo é uma imagem |
| `/api/floorplans/{id}` | GET, DELETE | `viewer` / `operator` | Uma planta com seus marcadores e o estado ao vivo de cada host |
| `/api/floorplans/{id}/image` | GET | `viewer` | A imagem da planta |
| `/api/floorplans/{id}/pins` | PUT | `operator` | Substitui o conjunto de marcadores |

O `ServeMux` do Go casa `/api/floorplans/` por prefixo e não extrai variável de
caminho, então o sufixo é resolvido por `floorPlanRouter` — que **estreita** os
métodos do wrapper: a rota aceita `GET`, `PUT` e `DELETE`, mas `/image` só
responde `GET` e `/pins` só `PUT`.

## Alertas

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/alerts` | GET | `viewer` | Alertas disparados, recortados por unidade (sem unidade só para acesso global). `status`: `open` (padrão), `acked`, `resolved` ou `all`; `limit` de 1 a 500 (padrão 100); `from` e `to` em RFC3339 sobre `created_at`. Cada linha traz chave, severidade, texto, origem (`server_id`, `site_id`, `rule_id`) e a entrega (`delivery`, `attempts`, `next_attempt_at`, `last_attempt_at`, `last_error`) |
| `/api/alerts/summary` | GET | `viewer` | `{"open":n,"acked":n,"falhou":n}` no alcance de quem pergunta. `falhou` conta alerta aberto cuja entrega desistiu |
| `/api/alerts/ack` | POST | `viewer` na unidade | Reconhece o alerta `?id=N`: grava `acked_at` e `acked_by`. Sessão de máquina recebe 403; fora do alcance, 404; já resolvido, 409. Auditado como `alert.ack` |
| `/api/alerts/resolve` | POST | **operador** na unidade | Resolve o alerta `?id=N`. Mesmos erros do `ack`, mais 403 para quem não é operador. Auditado como `alert.resolve` |
| `/api/alerts/rules` | GET, POST, PUT, PATCH, DELETE | `viewer` / **operador global** | Regras de alerta. O `GET` é recortado por unidade. `metric`: `cpu`, `mem`, `disk`, `load`, `temperature`, `net_rx`, `net_tx` ou `rtt`. `"enabled"` omitido no `POST` vale `true`; `false` é gravado como `false` |

## Ingestão

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/ingest/metrics` | POST | credencial `agent` | Push de métricas do agente. `cpu` e `load1` são **opcionais**: ausentes gravam `NULL` (a amostra ainda vale por memória, disco e rede) e nunca viram zero, então não disparam regra em falso; `0` explícito continua sendo medição. `net_rx_bps` e `net_tx_bps` (bytes/s) são opcionais; ausente ou negativo vira `NULL` |
| `/api/ingest/inventory` | POST | credencial `collector` | Push de inventário do coletor remoto |

Estas duas **não passam pelos wrappers comuns**: sem CORS de navegador, e a
conferência de método acontece dentro do próprio handler, não em `allowMethods`.
Os únicos middlewares são o teto de corpo e a auditoria.

A auditoria aqui registra **somente a recusa**, e isso não é esquecimento: são
milhares de push por minuto num parque de algumas centenas de hosts, e gravar
cada sucesso destruiria a tabela e afogaria o sinal. Token inválido, ao
contrário, é exatamente o que a auditoria existe para capturar.

Cada rota aceita um tipo de credencial. Credencial `agent` em
`/api/ingest/inventory`, ou `collector` em `/api/ingest/metrics`, recebe **403**
antes de o corpo ser lido, e o handler grava uma única linha de auditoria de
recusa, com o tipo apresentado e o exigido: `inventory.kind_mismatch` na rota de
inventário e `ingest.kind_mismatch` na de métricas. O middleware não grava uma
segunda linha para a mesma recusa.

O token compartilhado (`X-Agent-Token`, valor de `AGENT_INGEST_TOKEN`) é legado e
vem **desligado**. Apresentado com `ALLOW_LEGACY_INGEST_TOKEN` desligada, recebe
401 com a instrução de trocar o convite em `POST /api/enroll`, e o handler grava
uma única linha de recusa com o IP de origem: `ingest.legacy_token_disabled` ou
`inventory.legacy_token_disabled`. Ligado, é aceito nas duas rotas como antes, sem
tipo e com aviso no log a cada uso. Ver o ADR
[009](adr/009-token-compartilhado-desligado-por-padrao.md).

| Resposta | Quando |
|---|---|
| 401 | Credencial ausente, inválida ou revogada, ou token compartilhado com `ALLOW_LEGACY_INGEST_TOKEN` desligada |
| 403 | Credencial válida de outro tipo |
| 409 | Unidade declarada diferente da unidade da credencial (`*.site_mismatch`) |
| 413 | Inventário com mais de 5000 hosts ou corpo acima do teto |

## Identidade de dispositivo

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/enroll/tokens` | POST | **admin global** | Emite um convite de uso único para uma unidade. O valor sai **uma vez** |
| `/api/enroll` | POST | público | Troca o convite pela credencial própria do dispositivo |
| `/api/devices` | GET, DELETE | **admin global** | Lista e revoga credenciais |

`/api/enroll` é pública porque quem chama ainda não tem credencial — é o que vem
buscar. A proteção é o convite ser de uso único, de validade curta (24 h), e o
teto de corpo e o limite de tentativa do wrapper público valerem aqui.

O tipo da credencial vem do convite, não do corpo. Se o corpo trouxer `kind` e
ele for diferente do tipo do convite, a resposta é **409** e o convite **não é
consumido**: o dispositivo certo ainda pode usá-lo. Sem `kind` no corpo, vale o
do convite. Convite inexistente, vencido ou já usado continua respondendo 401.

Revogar marca `revoked_at`, não apaga a linha: o rastro de auditoria precisa
continuar apontando para um dispositivo que existiu.

## Autenticação

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/auth/login` | POST | público | Usuário e senha, devolve token de sessão |
| `/api/auth/logout` | POST | `viewer` | Invalida a sessão atual |
| `/api/auth/me` | GET | `viewer` | Quem está autenticado, com papel e concessões |
| `/api/users` | GET, POST, PATCH, DELETE | **admin global** | Gestão de usuários e concessões. O `POST` aceita `"active"`; omitido vale `true` |
| `/api/stream-ticket` | POST | `viewer` | Ticket de uso único para as rotas de SSE |

## Auditoria e saúde

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/audit` | GET | **admin global** | Log de auditoria, paginado e filtrável por ator, ação, resultado, unidade e intervalo |
| `/healthz` | GET | — | Liveness para o orquestrador. Sem credencial e sem tocar no banco |
| `/readyz` | GET | — | Prontidão: 200 `{"status":"ok"}` com o banco respondendo ao ping em até 2 s; 503 com o banco nulo ou fora do ar. O corpo também traz `alertas` (`ok`, `degradado` ou `desligado`), `alertas_detalhe` quando degradado, `logs_descartados` e `alertas_falhos` como **números**, e `degradado` com a lista de motivos (canal de alerta degradado, alerta com entrega falhou, log descartado). O status HTTP continua vindo só do banco |
| `/metrics` | GET | `viewer` | Contadores do processo em texto (`dockkeeper_*`): alertas enfileirados, entregues e falhos, logs descartados, sessões SSH abertas, reconexões, pânicos recuperados e migrações aplicadas Canal de alerta degradado **não** derruba a prontidão: só o banco decide o status HTTP. Sem credencial |

`/api/audit` é admin global porque a tabela mostra ação de **todas** as unidades.

---

## Convenções

**Erro.** Sempre JSON, com a mensagem em pt-BR. Corpo acima do teto responde
`413`; método não permitido, `405` com o cabeçalho `Allow`.

**404 em vez de 403 para recurso fora do alcance.** Quando a sessão não alcança o
servidor pedido, a resposta é `404`, não `403`. Confirmar a existência do recurso
a quem não pode vê-lo já é vazamento.

**Timeouts.** `ReadHeaderTimeout` de 10 s fecha Slowloris. `WriteTimeout` fica
zerado de propósito: as rotas de SSE mantêm a resposta aberta indefinidamente e
seriam cortadas no meio.
