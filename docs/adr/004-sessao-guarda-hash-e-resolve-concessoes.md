# 004 — A sessão guarda hash e resolve as concessões a cada requisição

## Contexto

As sessões viviam num mapa em memória. Reiniciar o painel derrubava todos os
logins, e mais de uma réplica era impossível: sessão aberta contra uma delas não
existia para a outra. Para um projeto que se propõe a ser instalado por
terceiros, as duas coisas são bloqueio.

Ao mover para a tabela `user_sessions`, duas perguntas apareceram: o que gravar
na linha, e o que fazer com as concessões por unidade.

## Decisão

**A chave primária é o SHA-256 do token, nunca o token.**

**`Session.Accesses` não vai para a tabela.** As concessões são relidas de
`user_site_accesses` a cada `Lookup`, sem cache.

## Consequência

### Sobre o hash

**A favor.** Um vazamento da tabela — backup, réplica de leitura, `SELECT` de
quem só deveria consultar — não entrega sessão ativa de ninguém. A busca continua
sendo lookup por chave primária, então não custa nada.

**Contra.** Nada de relevante. É a decisão fácil deste ADR.

Sobre não usar bcrypt aqui, ver [ADR 003](003-segredo-de-dispositivo-usa-sha256.md).

### Sobre as concessões

**A favor.** Copiá-las para a sessão no login congelaria a permissão no momento em
que a pessoa entrou. Revogar acesso de alguém deixaria de ter efeito até o
próximo login — o que, com TTL de 12 horas, é meio turno de trabalho com
permissão que já devia ter sumido. É exatamente o dado que a mudança existe para
manter fresco.

De brinde, conta desativada perde a sessão na requisição seguinte, mesmo sem
ninguém chamar a revogação explícita.

**Contra.** Duas leituras por requisição autenticada: uma em `users` por chave
primária, uma em `user_site_accesses` por índice. Não é de graça.

**Por que sem cache.** Um cache reintroduziria a janela de defasagem que a decisão
existe para fechar, só que menor e mais difícil de raciocinar. Duas leituras
indexadas não são o gargalo de um handler que já faz consultas bem mais pesadas.
Se aparecer em profiling, dá para acrescentar depois — o contrário não é verdade.

### Banco indisponível

`Lookup` devolve falha de autenticação, nunca sucesso, e **não apaga a sessão**: a
indisponibilidade passa e a sessão precisa existir quando o banco voltar. Só
`record not found` e conta desativada apagam.

`CreateSession` sem banco devolve `ErrSessionStore`, que o handler traduz em `500`
com log — não em `401`. Erro de infraestrutura não pode se disfarçar de credencial
inválida, ou o operador vai passar a tarde conferindo senha.

### Poda

Sessão vencida é apagada no login seguinte, no padrão do `ticketStore`. Elas só se
acumulam enquanto gente entra, então quem entra paga a conta e a rotina de
retenção não precisa conhecer a tabela.
