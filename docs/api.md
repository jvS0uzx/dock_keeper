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
| `/api/metrics/catalogo` | GET | `viewer` | O registro único de métricas: `[{nome, rotulo, unidade, tem_tendencia, escopo, em_regra}]`. `escopo` é `servidor`, `container` ou `ambos`; `tem_tendencia=false` significa que períodos acima de 7 dias respondem 400 no histórico; `em_regra` diz se a métrica vale em regra de alerta e em painel de dashboard. A interface monta seus seletores a partir daqui |
| `/api/metrics/live` | GET | `viewer` | Último estado conhecido de hosts, containers e balanceador. É o que o painel consulta em polling. `lb_window_sec` diz a janela, em segundos, em que `load_balancing[].requests_count` foi contado. Por servidor traz `cpu`, `load1`, `temperature_c`, `net_rx_bps`, `net_tx_bps` e `rtt_ms`, `null` quando não há medição |
| `/api/metrics/history` | GET | `viewer` | Série temporal de uma métrica, com janela. Lê a tendência agregada nas janelas longas. `metric`: `cpu`, `mem`, `disk`, `load`, `temperature`, `latency` (handshake SSH), `rtt` (ms), `net_rx` e `net_tx` (bytes/s). `latency` e todo histórico de container (`container_id`) não têm tendência: período acima de `METRIC_RETENTION_DAYS` (7 dias) responde **400** em vez de devolver 7 dias como se fossem 30 |
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
| `/api/servers` | GET, POST, PATCH, DELETE | **admin global** | Cadastro dos hosts monitorados. Cadastrar entrega acesso SSH root, por isso é admin. O `PATCH` renomeia e ajusta `user` e `port`; `host_ip` não muda, porque é a identidade da coleta. `{"aliases":[...]}` **substitui** a lista manual inteira (lista vazia remove todos; endereço coletado não é tocado). `{"absence_alert":true\|false}` marca a estação para o alerta de ausência. O `GET` e o `/api/metrics/live` devolvem `absence_alert`, `aliases` (só os manuais) e `addresses` (todos, com o `host_ip`) |
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
| `/api/alerts` | GET | `viewer` | Alertas disparados, recortados por unidade **no SQL**, antes do `limit` (sem unidade só para acesso global). `status`: `open` (padrão), `acked`, `resolved` ou `all`; `limit` de 1 a 500 (padrão 100); `from` e `to` em RFC3339 sobre `created_at`; `site_id`: id da unidade, `none` (só os sem unidade, exige acesso global) ou `all`/ausente (tudo o que a pessoa pode ver). Unidade fora do alcance responde 403; valor inválido, 400. Cada linha traz chave, severidade, texto, origem (`server_id`, `site_id`, `rule_id`, mais `server_name` e `site_name` resolvidos, `null` quando não há), a entrega (`delivery`, `attempts`, `next_attempt_at`, `last_attempt_at`, `last_error`) e o ciclo de vida (`renotify_count`, `last_notified_at`, `last_seen_at`) |
| `/api/alerts/summary` | GET | `viewer` | `{"open":n,"acked":n,"falhou":n}` no alcance de quem pergunta, contado no SQL e sem teto de linhas. Aceita o mesmo `site_id` da listagem. `falhou` conta alerta aberto cuja entrega desistiu |
| `/api/alerts/ack` | POST | **operador** na unidade | Reconhece o alerta `?id=N`: grava `acked_at` e `acked_by`. Alerta sem unidade exige operador global. Viewer e sessão de máquina recebem 403; fora do alcance, 404; já resolvido, 409. Auditado como `alert.ack` |
| `/api/alerts/resolve` | POST | **operador** na unidade | Resolve o alerta `?id=N`. Mesmos erros do `ack`, mais 403 para quem não é operador. Auditado como `alert.resolve` |
| `/api/alerts/rules` | GET, POST, PUT, PATCH, DELETE | `viewer` / **operador global** | Regras de alerta. O `GET` é recortado por unidade. `metric`: `cpu`, `mem`, `disk`, `load`, `temperature`, `net_rx`, `net_tx` ou `rtt`. `"enabled"` omitido no `POST` vale `true`; `false` é gravado como `false` |

## Ingestão

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/ingest/metrics` | POST | credencial `agent` | Push de métricas do agente. `cpu` e `load1` são **opcionais**: ausentes gravam `NULL` (a amostra ainda vale por memória, disco e rede) e nunca viram zero, então não disparam regra em falso; `0` explícito continua sendo medição. `net_rx_bps` e `net_tx_bps` (bytes/s) são opcionais; ausente ou negativo vira `NULL` |
| `/api/ingest/inventory` | POST | credencial `collector` | Push de inventário do coletor remoto. `report_interval_sec` (opcional) declara de quanto em quanto tempo o coletor envia; fica gravado na credencial e é a base do alerta de ausência. Sem ele o painel assume 900 s |

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
segunda linha para a mesma recusa. Credencial gravada **sem tipo** não passa em rota
nenhuma: tipo vazio só vale para o token compartilhado legado.

Recusa de autenticação tem teto por IP (`INGEST_RATE_MAX_UNAUTH`, 30 por janela), conferido
**antes** da autenticação nas duas rotas de ingestão; o `POST /api/enroll` já tinha o seu
(`INGEST_RATE_MAX_ENROLL`). Acima do teto a resposta é 429 e a auditoria grava uma linha do
bloqueio por janela, não uma por requisição: requisição anônima não enche mais a tabela.
O `host_ip` do agente e o IP de origem da auditoria saem do mesmo `clientIP` do limitador,
então atrás do nginx do compose a estação aparece com o endereço dela, não com o do proxy.

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
| `/api/auth/login` | POST | público | Identificador e senha, devolve token de sessão. O campo `username` aceita o nome de usuário **ou** o e-mail cadastrado. O limite de tentativas conta pela **conta** resolvida: errar pelo username e pelo e-mail soma no mesmo teto |
| `/api/auth/logout` | POST | `viewer` | Invalida a sessão atual |
| `/api/auth/me` | GET | `viewer` | Quem está autenticado, com papel e concessões |
| `/api/users` | GET, POST, PATCH, DELETE | **admin global** | Gestão de usuários e concessões. O `POST` aceita `"active"`; omitido vale `true`. `POST` e `PATCH` aceitam `"nome"` e `"email"`, devolvidos no `GET` e no `/api/auth/me`. **409** quando o username é o e-mail de outro usuário, ou o e-mail é o username de outro. **409** também quando a mudança tiraria o último administrador **global efetivo** ativo: apagar, desativar, rebaixar o `role` ou trocar `accesses` por concessão só de unidade. Administrador só de filial não conta como outro administrador |
| `/api/stream-ticket` | POST | `viewer` | Ticket de uso único para as rotas de SSE |

## Auditoria e saúde

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/audit` | GET | **admin global** | Log de auditoria, paginado e filtrável por ator, ação, resultado, unidade e intervalo |
| `/healthz` | GET | — | Liveness para o orquestrador. Sem credencial e sem tocar no banco |
| `/readyz` | GET | — | Prontidão: 200 `{"status":"ok"}` com o banco respondendo ao ping em até 2 s; 503 com o banco nulo ou fora do ar. Sem credencial o corpo traz só `status`, `alertas` (`ok`, `degradado` ou `desligado`) e os contadores `logs_descartados`, `alertas_falhos` e `alertas_sem_canal`. Com sessão válida ou token de máquina (`Authorization: Bearer`), traz também `alertas_detalhe` e `degradado` com a lista de motivos: o erro cru do canal cita o `TELEGRAM_CHAT_ID` e não é público. O status HTTP é o mesmo nos dois casos e continua vindo só do banco |
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

## Renomear servidor

`PATCH /api/servers?id=<uuid>` aceita `name`, `user` e `port`, todos opcionais, e devolve o
servidor atualizado.

| Situação | Resposta |
|---|---|
| Nome válido | 200 com o servidor |
| Corpo inválido, nome vazio, acima de 64 caracteres ou porta fora de 1-65535 | 400 |
| Nome repetido em outro servidor da mesma unidade | 409 |
| Id inexistente | 404 |

O `host_ip` não muda: ele é a identidade da coleta e a chave do cadastro. Para trocar o
endereço, remova e cadastre de novo.

Renomear **não derruba a coleta**: o `ServerManager` acompanha a sessão SSH pelo id do
servidor, que não muda. A consequência é que as mensagens de alerta disparadas por aquele
stream seguem citando o nome antigo até a próxima reconexão, quando o alvo é remontado a
partir do banco.

## Nome e e-mail de usuário

`nome` (até 120 caracteres) e `email` (até 160, guardado em minúsculas) são opcionais. O
e-mail é único entre as contas que o preenchem; contas sem e-mail continuam convivendo, o que
o índice parcial `idx_users_email` garante. O login aceita o nome de usuário ou o e-mail, e o
caminho de "não encontrado" continua comparando contra o hash falso, para não vazar por tempo
de resposta quais identificadores existem.

## Endereços do servidor

Cada servidor declara os próprios endereços, para que o painel pare de adivinhar de quem é um
upstream do nginx. O `GET /api/metrics/live` devolve, por servidor, `addresses: []` com o
`host_ip` e todos os endereços conhecidos, sem repetir.

De onde vêm:

| Origem | Como chega | Poda |
|---|---|---|
| `coletado` | `stream_metrics.sh` lê `ip -o -4 addr` e o agente de estação lê as interfaces pela gopsutil; ambos mandam `addresses` no payload | Sai depois de `ADDRESS_RETENTION_DAYS` (padrão 30) sem ser visto |
| `manual` | `PATCH /api/servers?id=` com `{"aliases":["100.100.0.11"]}` | Nunca é podado |

Loopback, link-local e IPv6 são descartados nas duas pontas. Interface virtual do Docker
(`veth`, `docker`, `br-`, `virbr`) fica de fora, pela mesma razão do RX/TX: não é endereço por
onde a máquina se apresenta na rede.

### Um endereço pertence a um servidor só

`POST /api/servers` responde **409** quando o `host_ip` já pertence a outro servidor, seja como
`host_ip` dele, seja como endereço coletado ou alias. A mensagem diz qual servidor e qual
unidade. Antes desta trava, cadastrar o mesmo IP com outro nome **renomeava** o servidor
existente em silêncio, que foi como o banco acabou com duas VPS-2. O `PATCH` aplica a mesma
regra aos `aliases`.

O alcance do conflito depende da faixa:

| Faixa | Conflito |
|---|---|
| Privada (RFC 1918) | Por unidade. Duas filiais podem ter `192.168.0.10` legitimamente |
| Pública | Global |
| `100.64/10` (CGNAT, usada por overlay como o Tailscale) | Global, porque é única na frota |

A recusa entra na auditoria como `server.create` com resultado `error`, que é como o painel já
classifica todo 409.

O cadastro é uma criação pura: depois de conferir o dono, o painel insere um servidor novo e
nunca reaproveita uma linha existente. É o que permite `192.168.0.10` existir na matriz e na
filial como dois servidores, sem um sobrescrever o nome e a unidade do outro.

Se a conferência do dono falhar por erro de banco, a resposta é **500** e nada é criado nem
gravado. Vale para o `POST`, para os `aliases` do `PATCH`, para a checagem de e-mail em uso de
`/api/users` e para o teto de 20 dashboards: nenhuma dessas regras segue em frente quando não
consegue consultar.

### Servidor removido libera o endereço

O `DELETE /api/servers?id=` apaga o servidor e os endereços dele (`server_addresses`) na mesma
transação. Servidor removido não é mais dono de nada: o mesmo `host_ip`, ou um antigo alias
dele, pode ser cadastrado de novo e responde 201. O recadastro cria um servidor novo, com id
novo; o registro antigo não é revivido, e o histórico dele sai pela retenção normal.

### O host também não toma endereço de ninguém

A regra de dono vale para o que o dispositivo declara, na ingestão do agente e na coleta por
SSH. Um endereço declarado que já pertence a outro servidor é **recusado**, não entra em
`server_addresses` e gera uma linha de auditoria `server.address_refused` (resultado `denied`),
com quem declarou, o endereço e o dono. O resto do envio é aceito normalmente: a amostra de
métrica é gravada e os demais endereços entram.

| Dono do endereço | Força | Quando deixa de segurar |
|---|---|---|
| `host_ip` de servidor SSH | Forte | Nunca: foi o administrador que cadastrou |
| Alias manual | Forte | Nunca |
| Endereço coletado de outro servidor | Fraca | Quando fica `15 min` sem ser reportado |
| `host_ip` observado de um agente | Fraca | Quando o agente fica `15 min` sem reportar |

A dona fraca existe por causa do DHCP: a estação que perdeu o IP para de reportá-lo, e a que
recebeu o mesmo IP passa a ser a dona depois da janela, levando o endereço com ela. Sem isso,
toda troca de lease viraria recusa por 30 dias.

Outras duas proteções do mesmo caminho: no máximo **32 endereços por envio**, com o excedente
descartado e registrado no log; e a mesma recusa gera auditoria **uma vez por hora** por par
servidor e endereço, para um dispositivo insistente não encher a tabela. O IP de origem da
conexão do agente, que o painel observa e não o dispositivo declara, é recusado em silêncio
quando já tem dono, porque várias estações atrás do mesmo NAT compartilham esse IP
legitimamente.

## Quem está atrás do balanceador

Topologia é estrutura, não fluxo: uma VPS continua atrás do balanceador num domingo sem
requisição nenhuma. Por isso o `GET /api/metrics/live` devolve, por servidor:

| Campo | O que é |
|---|---|
| `behind_lb` | booleano já resolvido, pronto para desenhar a malha |
| `behind_lb_origem` | `manual`, `trafego` ou `nenhum`, para a tela explicar de onde veio a classificação |

A resolução tem duas fontes, nesta ordem:

1. **Manual**, quando `servers.behind_lb` não é nulo. `PATCH /api/servers?id=` aceita
   `{"behind_lb": true}`, `false` ou `null`. O `null` devolve o servidor ao automático. É a
   saída para a VPS que entrou no balanceador hoje e ainda não recebeu request, e para tirar
   da malha quem saiu.
2. **Memória de tráfego**, quando o manual é nulo: algum endereço do servidor (`host_ip`,
   coletado ou alias) apareceu como upstream em `metric_load_balancers` nos últimos
   `LB_MEMBERSHIP_DAYS` (padrão 7).

A consulta compara o endereço sem a porta (`split_part(upstream_addr, ':', 1)`) e usa o índice
`idx_metric_lb_ts_upstream`, criado na migração 008.


### Estados de entrega do alerta

| `delivery` | O que significa |
|---|---|
| `pendente` | Na fila, esperando o despachante |
| `enviado` | Entregue no canal com sucesso |
| `falhou` | Esgotou `ALERT_MAX_ATTEMPTS`; o alerta continua aberto e visível |
| `falhou` (retomada) | Quando o canal volta a responder depois da última tentativa, o alerta aberto e visto nas últimas `ALERT_RESUME_HOURS` volta para `pendente` e ganha **uma** tentativa. Falhando de novo, espera o próximo sinal de canal de pé |
| `sem_canal` | Não há canal configurado. Nada foi entregue, e o alerta continua aberto. Quando o canal passa a existir, o que ainda está aberto e foi visto (`last_seen_at`) nas últimas `ALERT_RESUME_HOURS` volta para `pendente` |
| `dispensado` | O alerta foi resolvido antes de a primeira entrega acontecer. Nada foi enviado, e nada será: não se avisa de um problema que já acabou |

### Ciclo de vida do alerta

Um incidente é **um** registro. Enquanto a condição continua, a mesma linha é reaproveitada:

| Campo | O que guarda |
|---|---|
| `created_at` | Quando o incidente começou |
| `last_seen_at` | Última vez em que a condição foi observada (atualizado no máximo uma vez por minuto) |
| `last_notified_at` | Última entrega bem-sucedida no canal |
| `renotify_count` | Quantas vezes o mesmo incidente foi avisado de novo, uma a cada `ALERT_COOLDOWN` |

- A renotificação devolve a linha para `pendente`, atualiza o texto com o valor corrente e soma
  `renotify_count`. Oito horas de CPU alta são uma linha com `renotify_count` 15, não 16 linhas.
- Alerta reconhecido (`acked`) continua sendo o mesmo incidente e **para de renotificar**. Reconhecer
  é dizer "já vi".
- Incidente sem sinal de vida há mais de `ALERT_RESUME_HOURS` não segura a chave: a próxima
  ocorrência abre um alerta novo. O antigo fica aberto até alguém resolver.
- Incidente novo dentro do `ALERT_COOLDOWN` do último aviso da chave é gravado na hora, para o
  painel mostrar, e o aviso no canal espera o cooldown vencer. Resolvido antes disso, vira `dispensado`.
- A mensagem de recuperação é uma linha própria, de chave `<chave>:recuperacao`, que **nasce
  `resolved`**: ela é um aviso, não um problema aberto. Só existe quando o incidente chegou a ser
  avisado no canal, e herda `server_id`, `site_id` e `rule_id` do incidente.
- O despachante não entrega alerta já resolvido, com exceção da mensagem de recuperação.

## Prontidão em dois caminhos

O mesmo handler responde em dois lugares, com corpo e código idênticos:

| Caminho | Para quem | CORS |
|---|---|---|
| `/readyz` | Orquestrador, compose, healthcheck | Não passa pela cadeia de CORS, e esses clientes não precisam |
| `/api/readyz` | **A interface** | Passa pela cadeia pública, com a allowlist de `ALLOWED_ORIGINS` |

A interface precisa usar `/api/readyz`. O `/readyz` direto é barrado pelo navegador em
desenvolvimento (origem diferente) e responde 404 em produção, porque o nginx do frontend só
faz proxy de `/api/`. Era por isso que a faixa de degradação nunca aparecia.
