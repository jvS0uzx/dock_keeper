# 008 — SSE em vez de WebSocket, com ticket de uso único

## Contexto

O painel precisa transmitir ao navegador o que chega em tempo real: `docker
logs -f`, o log de autenticação do host.

WebSocket é a escolha reflexa. E o README antigo chegou a afirmar que o projeto o
usava — não usa, nunca usou; não há uma linha de WebSocket no código.

## Decisão

Server-Sent Events (`text/event-stream`), autorizados por **ticket de uso único**
em vez do token permanente.

## Consequência

### Sobre o SSE

**A favor.** O fluxo é unidirecional: o navegador não precisa mandar nada de volta
por esse canal. SSE é HTTP comum, reconecta sozinho, atravessa proxy sem
negociação de upgrade, e não acrescenta dependência.

**Contra.** Um stream por aba, e o navegador limita conexões simultâneas por
origem. Não morde hoje porque o painel abre um stream por vez.

**Cuidado com o proxy.** Atrás de nginx sem `proxy_buffering off`, os eventos são
segurados e **a tela fica parada sem erro nenhum aparecer**. O handler manda
`X-Accel-Buffering: no`, que resolve para o nginx, mas a configuração explícita
está documentada em `backend/deploy/README.md` porque nem todo proxy respeita o
cabeçalho.

**Cuidado com o `WriteTimeout`.** Fica zerado no servidor de propósito: as rotas
de SSE mantêm a resposta aberta indefinidamente e seriam cortadas no meio.

**Cuidado ao envolver o `ResponseWriter`.** `startSSE` faz type assertion para
`http.Flusher`. Qualquer envelope na cadeia precisa repassar `Flush()`, senão o
Go bufferiza e o painel não recebe nada. Foi por isso que o middleware de
auditoria não entrou no wrapper de stream, e por isso o envelope dele implementa
`Flush` mesmo sem precisar hoje.

### Sobre o ticket

**O problema.** `EventSource` não permite definir cabeçalho. A credencial teria
que ir na query string — onde acabaria no access log do proxy e no histórico do
navegador, os dois lugares que ninguém rotaciona.

**A solução.** `POST /api/stream-ticket` devolve um ticket de uso único, válido
por 30 segundos, que vai na URL sem risco: quando ele vazar para o log, já não
vale mais.

**O detalhe que importa.** O ticket **carrega a sessão de quem o pediu**. Sem
isso, ele autorizava o stream com privilégio de máquina, e as rotas de SSE
concediam admin global a qualquer usuário — furo que existiu e foi corrigido. Hoje
o stream corre com o papel e o alcance da pessoa.

O cabeçalho continua valendo para clientes que conseguem enviá-lo (`curl`,
agente), então o ticket é uma alternativa, não uma substituição.
