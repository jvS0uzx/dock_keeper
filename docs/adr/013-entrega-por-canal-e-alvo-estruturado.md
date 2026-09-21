# ADR 013 — Entrega por canal, e o alerta deixa de ser um texto de Telegram

Data: 2026-09-20
Estado: aceito

## Contexto

O [ADR 011](011-alerta-em-fila-persistida.md) tirou a entrega do caminho de quem
detecta o problema, mas manteve duas premissas que o Telegram tinha imposto desde
o começo e que a evolução do produto desmentiu.

**O alerta era um texto pronto.** Quem detectava montava a frase final e chamava
`Deliver(msg string)`, função de pacote em `telegram.go`. Severidade, host,
métrica e limiar existiam só dissolvidos dentro da string. Qualquer canal novo
receberia uma frase formatada para outro destino, e nenhum consumidor podia
filtrar, agrupar ou correlacionar por alvo sem fazer análise de texto.

**A entrega era um estado do alerta, não da tentativa.** `delivery`, `attempts`,
`next_attempt_at`, `last_attempt_at` e `last_error` eram colunas da linha em
`alerts`. Com um canal só isso funciona. Com dois, o retry de um zera o contador
do outro, um canal fora do ar segura o alerta que o outro já entregou, e não há
como dizer "foi para o webhook, não foi para o Telegram".

A terceira força é o alvo. Enquanto o objeto monitorado era sempre uma VPS,
`server_id` bastava. Container já não cabia — o painel afirmava "rodando" sobre
container em ciclo de reinício justamente porque nada no alerta distinguia o
container do host que o hospeda. Interface de rede não cabe de jeito nenhum: o
alvo ali é o par (dispositivo, porta), e a métrica é da porta.

## Decisão

**O alvo do alerta é estruturado e vive em colunas.** `alerts` ganhou
`alvo_tipo` (`host`, `container`, `servico`, `interface`, com CHECK),
`alvo_id`, `alvo_nome`, `metrica`, `valor`, `limiar` e `unidade`. Todos nuláveis:
alerta sem número observado deixa `valor` nulo, e nunca zero. `alert.Entrada`
espelha esses campos.

**A entrega é uma linha por par (alerta, canal).** A tabela `alert_deliveries`
tem índice único em `(alert_id, canal)` e carrega o `status`, o contador de
tentativas, o backoff e o último erro daquele canal. É a verdade da entrega.

**As colunas antigas em `alerts` viraram espelho consolidado.** A API e o painel
continuam lendo `delivery`, `attempts`, `next_attempt_at` e `last_error` como
antes; o despachante recalcula esses valores a partir das linhas: tentativas é o
máximo entre canais, a próxima tentativa é a mais próxima entre os pendentes, e
`delivery` fica pendente enquanto algum canal deve, depois `sem_canal`, depois
`falhou`, e só é `enviado` quando todos os canais ativos entregaram. Com um canal
configurado, a consolidação reduz exatamente ao comportamento anterior.

**O canal é uma interface, e o texto nasce na entrega.** `Canal` tem `Nome()`,
`Ativo()` e `Entregar(database.Alert)`. Telegram é a primeira implementação;
webhook (POST de JSON com o alvo estruturado) e ntfy (POST do texto num tópico)
são a segunda e a terceira. Quando `Entrada.Text` vem vazio, o texto é renderizado
a partir do alvo na hora de entregar, por canal. Quando vem preenchido, é
respeitado — é o caminho dos pontos ainda não migrados.

**`Ativo()` é consultado a cada despacho**, e não congelado na inicialização, para
que um canal configurado depois entre sem reinício do processo.

**E-mail e SMS ficam fora**, por decisão do dono, registrada também no plano de
telemetria de rede. Quem precisa de e-mail tem webhook.

## Consequências

- Dois canais falham de forma independente. O que entregou fica com uma tentativa
  enquanto o vizinho caminha para o teto de `ALERT_MAX_ATTEMPTS`.
- `last_notified_at` só avança quando algum canal entregou *naquela passada*. Sem
  isso, um webhook falhando empurraria o cooldown a cada ciclo e a renotificação
  nunca aconteceria.
- Renotificação e retomada precisam cascatear para as linhas de canal: retomar um
  alerta preso sem zerar o contador das linhas em `falhou` não reenviaria nada,
  porque o teto de tentativas já estava estourado.
- O alvo estruturado abre filtro e agrupamento por alvo sem análise de texto, que
  é o pré-requisito da supressão por causa raiz e da tela de incidente.
- Duas dívidas ficam explícitas. `Status()` continua sendo a saúde do Telegram
  especificamente, e a retomada de alertas presos usa essa saúde como gatilho para
  todos os canais — com webhook ativo e Telegram fora, o critério é impreciso.
  Generalizar saúde por canal é desenho novo, não feito aqui.
- A migração 014 copia o estado de entrega de cada alerta existente para uma linha
  de canal `telegram`, então nenhum alerta em voo fica órfão. As colunas antigas
  não foram removidas: remover é uma migração futura, depois que nada mais as ler.
