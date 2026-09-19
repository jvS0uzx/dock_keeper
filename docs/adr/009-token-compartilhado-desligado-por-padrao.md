# 009 — O token compartilhado de ingestão vem desligado

## Contexto

Antes da identidade por dispositivo (S7), um único `AGENT_INGEST_TOKEN`
autenticava todos os agentes e coletores de todas as unidades. Quem tinha o valor
declarava qualquer unidade no corpo do envio, forjava inventário de outra filial e
injetava métrica que dispara ou silencia alerta.

O S7 trouxe o substituto: convite de uso único, credencial própria por
dispositivo e unidade tirada da credencial. O token compartilhado continuou aceito,
com aviso no log, para as estações instaladas antes da migração não pararem de uma
vez.

Aceito por padrão, porém, o caminho antigo nunca fecha: uma instalação nova herda
um segredo compartilhado que ninguém pediu, e o recorte por unidade depende de
todo mundo ter migrado.

## Decisão

O `X-Agent-Token` só é aceito com `ALLOW_LEGACY_INGEST_TOKEN=true`. O padrão é
`false`.

Desligado, o envio com esse cabeçalho recebe **401**, com mensagem que manda
trocar o convite em `POST /api/enroll`, antes de o valor ser comparado com o
`AGENT_INGEST_TOKEN`. O handler grava uma única linha de recusa
(`ingest.legacy_token_disabled` ou `inventory.legacy_token_disabled`) com o IP de
origem, no padrão do [002](002-ingestao-audita-so-recusa.md).

## Consequência

**A favor.** Instalação nova nasce só com credencial por dispositivo. O recorte
por unidade deixa de ter uma porta lateral, e o log de auditoria diz de onde ainda
chegam envios no formato antigo.

**Contra — estação antiga.** Agente ou coletor que só tem `AGENT_TOKEN` ou
`COLLECTOR_TOKEN` configurado para de reportar na atualização do painel: recebe
401, trata como credencial recusada e não repete o envio. A máquina some das
telas de métrica e de inventário até ser migrada.

**Migração.** Para cada estação: emitir convite em `POST /api/enroll/tokens` (tela
de Dispositivos), colocar o valor em `AGENT_ENROLL_TOKEN` ou
`COLLECTOR_ENROLL_TOKEN` e reiniciar o serviço. A consulta
`action LIKE '%.legacy_token_disabled'` na auditoria lista os IPs que faltam.

**Volta atrás.** `ALLOW_LEGACY_INGEST_TOKEN=true` restaura o comportamento
anterior sem mexer em dado. Serve de janela de transição, não de configuração
permanente.
