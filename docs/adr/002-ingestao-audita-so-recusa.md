# 002 — A ingestão audita só a recusa

## Contexto

O middleware de auditoria cobre toda rota não-`GET`, o que garante cobertura:
handler novo que esqueça de auditar não existe, porque o registro está no wrapper
que ele já usa.

`POST /api/ingest/metrics` e `POST /api/ingest/inventory` recebem push de **todo
agente e coletor instalado, a cada poucos segundos**. Com 500 hosts reportando a
cada 5 segundos são 6000 linhas por minuto.

## Decisão

As duas rotas de ingestão usam `auditOnlyDenied`: registram `denied`, nunca `ok`.

## Consequência

**A favor.** Auditar o sucesso destruiria a tabela — 8,6 milhões de linhas por
dia, num parque médio — e afogaria o sinal: as linhas que interessam ficariam
enterradas sob push de rotina. Token inválido, ao contrário, é exatamente o que a
auditoria existe para capturar.

**Contra.** Não há registro de "este dispositivo reportou às 14h32". Quem quiser
essa informação usa `device_credentials.last_seen_at`, que é atualizado a cada
envio, ou a própria série de métricas — que é o registro natural disso.

**Cuidado.** A ausência dessas linhas **parece esquecimento** e não é. Está
comentada em `server.go` e documentada em [`../api.md`](../api.md) justamente
porque a primeira reação de quem lê o código é achar que faltou cobertura.

`ingest.site_mismatch` — dispositivo com credencial válida declarando outra
unidade — é gravada pelo próprio handler, com detalhe próprio. É o sinal mais
direto de comprometimento que o sistema produz, e não podia depender do
middleware genérico.
