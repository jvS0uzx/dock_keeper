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
| dispositivo | Credencial de dispositivo ou `AGENT_INGEST_TOKEN` |

"Global" quer dizer concessão sem unidade. Ver
[`autenticacao.md`](autenticacao.md).

---

## Métricas

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/metrics/live` | GET | `viewer` | Último estado conhecido de hosts, containers e balanceador. É o que o painel consulta em polling |
| `/api/metrics/history` | GET | `viewer` | Série temporal de uma métrica, com janela. Lê a tendência agregada nas janelas longas |
| `/api/logs/search` | GET | `viewer` | Busca no histórico de linhas de log, recortada por unidade |

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
| `/api/sites` | GET, POST, DELETE | `viewer` / `operator` | Unidades (filiais) |

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
| `/api/alerts/rules` | GET, POST, PUT, PATCH, DELETE | `viewer` / **operador global** | Regras de alerta. O `GET` é recortado por unidade |

## Ingestão

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/ingest/metrics` | POST | dispositivo | Push de métricas do agente |
| `/api/ingest/inventory` | POST | dispositivo | Push de inventário do coletor remoto |

Estas duas **não passam pelos wrappers comuns**: sem CORS de navegador, e a
conferência de método acontece dentro do próprio handler, não em `allowMethods`.
Os únicos middlewares são o teto de corpo e a auditoria.

A auditoria aqui registra **somente a recusa**, e isso não é esquecimento: são
milhares de push por minuto num parque de algumas centenas de hosts, e gravar
cada sucesso destruiria a tabela e afogaria o sinal. Token inválido, ao
contrário, é exatamente o que a auditoria existe para capturar.

## Identidade de dispositivo

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/enroll/tokens` | POST | **admin global** | Emite um convite de uso único para uma unidade. O valor sai **uma vez** |
| `/api/enroll` | POST | público | Troca o convite pela credencial própria do dispositivo |
| `/api/devices` | GET, DELETE | **admin global** | Lista e revoga credenciais |

`/api/enroll` é pública porque quem chama ainda não tem credencial — é o que vem
buscar. A proteção é o convite ser de uso único, de validade curta (24 h), e o
teto de corpo e o limite de tentativa do wrapper público valerem aqui.

Revogar marca `revoked_at`, não apaga a linha: o rastro de auditoria precisa
continuar apontando para um dispositivo que existiu.

## Autenticação

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/auth/login` | POST | público | Usuário e senha, devolve token de sessão |
| `/api/auth/logout` | POST | `viewer` | Invalida a sessão atual |
| `/api/auth/me` | GET | `viewer` | Quem está autenticado, com papel e concessões |
| `/api/users` | GET, POST, PATCH, DELETE | **admin global** | Gestão de usuários e concessões |
| `/api/stream-ticket` | POST | `viewer` | Ticket de uso único para as rotas de SSE |

## Auditoria e saúde

| Rota | Métodos | Exige | O que faz |
|---|---|---|---|
| `/api/audit` | GET | **admin global** | Log de auditoria, paginado e filtrável por ator, ação, resultado, unidade e intervalo |
| `/healthz` | GET | — | Liveness para o orquestrador. Sem credencial |

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
