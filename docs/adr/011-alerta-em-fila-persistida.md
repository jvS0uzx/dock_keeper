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
  controlada por `ALERT_NOTIFY_RECOVERY`. *(Revisto em 2026-09-19; ver a seção
  seguinte.)*

## Revisão de 2026-09-19 — o ciclo de vida do incidente

A primeira versão tratava cada disparo como linha nova. A auditoria de 19/09 mostrou o
custo: oito horas de CPU alta viravam 16 alertas `open`, só regra do motor resolvia o
próprio alerta, a mensagem de recuperação aparecia como problema aberto, e um alerta
preso em `sem_canal` calava a chave para sempre. O desenho atual:

- **Um incidente, um alerta.** `Enqueue` procura o alerta não resolvido da chave visto
  (`last_seen_at`) dentro de `ALERT_RESUME_HOURS` e o reaproveita. A renotificação a
  cada `ALERT_COOLDOWN` devolve a **mesma** linha para `pendente`, soma
  `renotify_count` e, entregue, grava `last_notified_at`. `Notify` deixou de existir:
  todo chamador usa `Enqueue`, com `server_id` e `site_id`.
- **Alerta antigo não cala a chave.** Incidente sem sinal de vida além da janela, ou
  resolvido, não é mais o incidente corrente: a ocorrência seguinte abre alerta novo.
- **Reconhecer silencia.** Alerta `acked` continua sendo o incidente e para de
  renotificar. Desde a mesma revisão, reconhecer exige operador na unidade do alerta.
- **Incidente novo dentro do cooldown** é gravado na hora, para o painel mostrar, com
  `next_attempt_at` no fim do cooldown. O cooldown segura o canal, não o registro.
- **Todo gatilho resolve o próprio alerta**, com evidência positiva de que a condição
  voltou (a tabela está em `docs/metricas.md`). Silêncio não resolve.
- **Recuperação condicionada.** A mensagem de recuperação só existe se o incidente
  chegou a ser avisado no canal. Ela é uma linha própria (`<chave>:recuperacao`) que
  **nasce `resolved`**, herda a origem do incidente, e é a única linha resolvida que o
  despachante entrega.
- **`dispensado`.** Alerta resolvido antes da primeira entrega não é enviado: avisar de
  um problema que já acabou é ruído. A entrega fica `dispensado`, e não `pendente` para
  sempre nem `enviado`, que seria mentira. Se já havia sido avisado antes e foi
  resolvido no meio de uma renotificação, volta para `enviado`.
- **`falhou` é retomado.** Dentro de `ALERT_RESUME_HOURS`, cada sinal de canal de pé
  posterior à última falha dá ao alerta uma tentativa. Sem sinal novo, sem tentativa.
- **Piso único.** `ALERT_MIN_SEVERITY` é aplicado dentro de `Enqueue`, então vale para
  regra e para gatilho, e o aviso de "abaixo do mínimo" sai uma vez por cooldown.

A entrega continua sendo **ao menos uma vez**, e a renotificação herda a mesma
garantia: a linha renotificada passa pela mesma reserva com prazo.

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
- Custo: uma tabela a mais. Com um incidente por linha ela cresce com o número de
  incidentes, não com a duração deles; alerta resolvido é podado por
  `ALERT_RETENTION_DAYS`.
- O despachante é mais um ponto que precisa estar vivo. Ele roda sob `safego`, que
  o reinicia depois de pânico, e o atraso aparece como alerta `pendente` antigo.

## Alternativas descartadas

- **Manter o envio síncrono e só repetir na hora.** Repetir dentro do tick piora o
  bloqueio que motivou o ADR, e não resolve o processo reiniciado no meio.
- **Fila em memória.** Não sobrevive a restart, que é exatamente quando o alerta
  costuma ser perdido, e não serve para mostrar o incidente na tela.
- **Broker externo.** Um Redis ou RabbitMQ só para isso não se paga num painel de
  instância única que já tem Postgres.
