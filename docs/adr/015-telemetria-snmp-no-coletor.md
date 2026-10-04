# ADR 015 — A telemetria SNMP vive no coletor, e o painel só guarda

Data: 2026-10-03
Estado: aceito

## Contexto

O inventário de rede sabia que um switch existia e quais portas TCP ele abria,
mas não sabia nada do que passava por ele. A direção de produto é monitorar a
rede da filial, e o primeiro passo visível é ver o tráfego de cada interface dos
equipamentos gerenciáveis.

O painel não alcança a rede da filial. Quem alcança é o coletor remoto
(`dockkeeper_collector`), que já roda lá com credencial `collector` e já envia o
inventário. SNMP é UDP, de rede local, com comunidade que não deve sair da
filial.

## Decisão

**O SNMP vive no coletor.** A sessão SNMP, a comunidade, o timeout e o walk de
`IF-MIB` ficam no coletor. O painel nunca fala SNMP e nunca recebe a comunidade.

**O delta é calculado no coletor.** Os contadores de interface são cumulativos e
dão a volta (32 bits em interface rápida dá a volta em segundos). Só quem leu as
duas amostras sabe o intervalo real entre elas e se houve reinício
(`sysUpTime` voltou) ou volta do contador. O coletor manda taxa em bps e o
número de erros e descartes **do ciclo**; quando não dá para calcular, manda
`null`. O painel nunca transforma `null` em zero, e recusa bps negativo com 400,
porque isso só acontece com defeito no coletor.

**O contrato é versionado.** O corpo traz `schema: 1`, e o painel recusa outro
número com 400. Mudança de formato vira `schema: 2` com o painel aceitando os dois
durante a transição, em vez de um campo que muda de sentido em silêncio.
Campo desconhecido é ignorado, para o coletor poder mandar coisa nova antes do
painel saber ler.

**A interface é identificada por `if_name`.** O `ifIndex` não é estável entre
reinícios em boa parte dos equipamentos. A reconciliação casa por nome dentro do
host e só usa o índice quando o nome vem vazio; renumeração preserva o histórico.
Interface que some do envio não é apagada. O banco garante a unicidade do nome
por host só quando o nome existe (índice único parcial).

**Leitura bruta por 72 h, sem tendência no D1.** `metric_network_interfaces`
guarda a leitura de cada ciclo e é podada por `NETWORK_METRIC_RETENTION` (padrão
`72h`) no mesmo laço e com o mesmo lote da poda das outras métricas. Não há
rollup horário ainda.

**SNMP v2c agora, v3 depois.** v2c cobre o parque atual e é o que todo
equipamento gerenciável fala. v3 (autenticação e privacidade) entra no coletor
sem mudar o contrato, porque a credencial SNMP nunca chega ao painel.

## Consequências

- O painel continua sem nenhuma porta UDP nem dependência SNMP, e a comunidade
  não sai da filial.
- Coletor e painel evoluem separados, presos só ao contrato `schema: 1`. Um
  painel antigo recusa um coletor novo com 400 explícito em vez de gravar errado.
- Um lote inteiro é recusado por um único IP inválido ou bps negativo. É
  proposital: o defeito aparece no log do coletor em vez de virar buraco
  silencioso, ao custo de perder aquele ciclo.
- Com 72 h e sem tendência, não dá para comparar a semana passada nem planejar
  capacidade. O rollup horário de interface é a próxima pendência do bloco.
- Interface que deixou de existir fica na tabela até o host ser podado. Com
  módulos trocados com frequência a lista cresce; poda por idade de interface é
  pendência.
- O `last_seen` do host passa a ser renovado pela leitura SNMP. Switch que não
  abre porta TCP nenhuma deixa de sair do inventário por `HOST_RETENTION_DAYS`
  enquanto responder SNMP.
