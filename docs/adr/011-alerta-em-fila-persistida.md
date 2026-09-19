# ADR 011 — O alerta vira linha no banco antes de virar mensagem

Data: 2026-09-18
Estado: aceito

## Contexto

Até aqui o caminho do alerta era síncrono. `alert.Notify` gravava o cooldown e
chamava `Send`, que fazia o `POST` no Telegram na mesma goroutine de quem
detectou o problema: o tick do motor de regras, o `onError` do supervisor SSH, o
vigia de SSL. Duas consequências, as duas medidas na avaliação de arquitetura de
18/09:

- **Alerta perdido sem sinal.** O cooldown era gravado *antes* do envio e `Notify`
  devolvia `true` mesmo quando o `POST` falhava. Com o Telegram fora por dois
  minutos durante um incidente, o aviso sumia: o motor marcava a regra como
  anunciada e o alerta só voltaria depois da métrica se recuperar e quebrar de
  novo, ou depois dos 30 minutos de cooldown. O log era o único vestígio.
- **Quem alerta espera pela API do Telegram.** O cliente HTTP tem timeout de 10 s.
  Uma rajada de 30 alertas com a API lenta atrasava o tick do motor em minutos, e
  o mesmo envio bloqueava a reconexão SSH que tinha acabado de falhar.

## Decisão

`alert.Notify` (e o `alert.Enqueue` com metadados) **persiste** o alerta na tabela
`alerts` com `status=open` e `delivery=pendente`, e devolve na hora. Um
despachante de fundo, supervisionado pelo `safego`, faz a entrega.

- A reserva do lote usa `SELECT ... FOR UPDATE SKIP LOCKED` e grava um prazo em
  `next_attempt_at` antes de soltar a transação, então dois processos nunca
  entregam o mesmo alerta, e a chamada HTTP acontece fora da transação.
- Falha de entrega incrementa `attempts`, grava `last_error` e agenda a próxima
  tentativa com espera dobrando de 1 min até o teto de 1 h. Acima de
  `ALERT_MAX_ATTEMPTS` (padrão 8) a entrega é marcada `falhou` — e o alerta
  **continua `open`**, visível no painel e no resumo.
- O cooldown passou a olhar a última entrega **com sucesso** da mesma chave, não a
  última tentativa. Enquanto existe alerta pendente da mesma chave, nada novo é
  enfileirado: durante uma queda do Telegram a fila não incha.
- A recuperação resolve o alerta correspondente e enfileira uma mensagem `[OK]`,
  controlada por `ALERT_NOTIFY_RECOVERY`.

## Entrega é ao menos uma vez

Se o processo cair depois de o Telegram aceitar a mensagem e antes de a linha ser
marcada como `enviado`, a reserva vence e o alerta é entregue de novo. É escolha
deliberada: repetir um aviso incomoda, perder um aviso durante incidente é o que
esta mudança existe para impedir. O operador precisa saber que alerta repetido
depois de um restart não é bug.

## Consequências

- O motor pode marcar a regra como anunciada assim que o alerta é **enfileirado**,
  porque a entrega agora tem retry e deixa rastro.
- O incidente fica visível no painel mesmo sem canal: `GET /api/alerts` e
  `/api/alerts/summary` mostram `open`, `acked` e quantos falharam na entrega.
  Alerta não depende mais de o Telegram estar de pé para existir.
- Ack e resolve são registrados com autor e auditados.
- Custo: uma tabela a mais, que cresce com o volume de alerta e ainda não tem
  poda própria; a retenção entra junto com o trabalho de particionamento.
- O despachante é mais um ponto que precisa estar vivo. Ele roda sob `safego`, que
  o reinicia depois de pânico, e o atraso aparece como alerta `pendente` antigo.

## Alternativas descartadas

- **Manter o envio síncrono e só repetir na hora.** Repetir dentro do tick piora o
  bloqueio que motivou o ADR, e não resolve o processo reiniciado no meio.
- **Fila em memória.** Não sobrevive a restart, que é exatamente quando o alerta
  costuma ser perdido, e não serve para mostrar o incidente na tela.
- **Broker externo.** Um Redis ou RabbitMQ só para isso não se paga num painel de
  instância única que já tem Postgres.
