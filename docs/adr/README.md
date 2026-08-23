# Decisões de arquitetura

Decisões que até agora só viviam como comentário no código. Cada uma tem
contexto, decisão e consequência — incluindo a consequência ruim, que é a parte
que costuma sumir quando alguém revisita a escolha meses depois.

| # | Decisão |
|---|---|
| [001](001-auditoria-nao-derruba-requisicao.md) | Falha de auditoria não derruba a requisição |
| [002](002-ingestao-audita-so-recusa.md) | A ingestão audita só a recusa |
| [003](003-segredo-de-dispositivo-usa-sha256.md) | Segredo de dispositivo usa SHA-256, não bcrypt |
| [004](004-sessao-guarda-hash-e-resolve-concessoes.md) | A sessão guarda hash e resolve as concessões a cada requisição |
| [005](005-indice-do-inventario-usa-coalesce.md) | O índice do inventário usa `COALESCE(site_id, 0)` |
| [006](006-auditoria-mora-na-fronteira.md) | A auditoria mora na fronteira, não nos handlers |
| [007](007-404-em-vez-de-403.md) | Recurso fora do alcance responde 404, não 403 |
| [008](008-sse-em-vez-de-websocket.md) | SSE em vez de WebSocket, com ticket de uso único |
