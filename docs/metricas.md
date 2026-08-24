# Métricas

## Nulo não é zero

É o fio condutor de tudo aqui. O sistema gravava **zero onde não tinha medição**,
e zero é um número como qualquer outro na tela: o gráfico de temperatura de uma
VPS sem sensor era uma reta no chão, indistinguível de uma máquina realmente a
0 °C.

Hoje o campo é ponteiro e o não medido é `NULL`, que a API serializa como `null`
e a tela escreve "sem sensor" em vez de desenhar.

**Migração é assimétrica.** O `AutoMigrate` do GORM não afrouxa a nulidade de uma
coluna que já existe, e as linhas gravadas antes da correção **seguem com 0**. Só
o dado novo distingue os dois casos. Um gráfico de 30 dias pode mostrar a reta no
chão até a retenção passar por cima.

## Temperatura

`MetricServer.TemperatureC` é a **maior** temperatura reportada pelos sensores da
máquina, em °C.

`NULL` significa uma de duas coisas: a fonte não mede, ou a máquina não tem sensor
legível — VM e container em geral não têm.

Há um tratamento extra na ingestão do agente: **zero recebido vira nulo**. Agente
antigo mandava `0` quando não achava sensor, e gravar esse zero fazia o painel
exibir "0 °C" como se fosse leitura. Nenhuma máquina em operação está a 0 °C, e o
custo de descartar uma leitura real de zero é aceitável perto do custo de mentir.

Na tendência agregada, `AVG(NULLIF(temperature_c, 0))` devolve `NULL` quando
nenhuma amostra da hora tinha sensor — por isso `TemperatureAvg` e
`TemperatureMax` também são ponteiros. `MemPercentAvg` e `DiskPercentAvg` pelo
mesmo motivo: o divisor é anulado quando o host reportou total zero.

## Handshake SSH — não é latência

`MetricServer.SSHHandshakeMs` é o tempo de **abrir a sessão SSH inteira**: TCP
mais troca de chaves, medido ao iniciar a coleta.

**Não é RTT de rede**, e fica uma ordem de grandeza acima dele — 1000 a 1400 ms
nas VPS onde foi medido, contra alguns milissegundos de ping.

O campo se chamava `PingLatencyMs` e o painel rotulava "Latência", o que induzia o
operador a ler o número como latência de rede e a diagnosticar problema de rede
onde havia custo de handshake.

A coluna no banco continua `ping_latency_ms` **de propósito**: renomeá-la faria o
`AutoMigrate` criar uma coluna nova e abandonar todo o histórico já gravado.

`NULL` quando a fonte não mede — o agente de push não abre sessão SSH, então
nunca preenche este campo.

RTT de verdade exigiria um prober separado, ICMP ou TCP periódico, que **não
existe** no projeto.

## Janela de "online"

Um host aparece online enquanto a última métrica dele couber na janela. A janela
**não é fixa**: sai do intervalo que o próprio agente informou
(`database.LiveWindowFor`).

```
janela = max(3 × report_interval_sec, 30s)
```

Três ciclos de tolerância, para um atraso pontual não derrubar o host da tela.

| `report_interval_sec` | Janela |
|---|---|
| 0 (desconhecido) | 30 s |
| 5 | 30 s (piso) |
| 15 | 45 s |
| 60 | 180 s |
| 120 | 360 s |

**Por que não é fixa.** Com janela fixa de 30 s, todo agente configurado com
`AGENT_INTERVAL` maior aparecia permanentemente offline mesmo reportando
certinho. E o motor de regras tinha uma segunda janela fixa, de 60 s, com efeito
pior: um agente com intervalo de 120 s **nunca** tinha métrica considerada
recente, então **nenhuma regra disparava para ele** — e se ele fosse dependência
de outras regras, todos os filhos ficavam suprimidos. O sintoma era silêncio, que
ninguém percebe.

Por isso a função mora em `internal/database`, ao lado de
`Server.ReportIntervalSec`: é a mesma definição de "recente" para dois
consumidores que não podem depender um do outro — o painel, que decide o rótulo
de online, e o motor de regras, que decide se a métrica é fresca o bastante para
avaliar. Enquanto estava duplicada, divergiu.

Intervalo desconhecido — coleta por SSH, cujo ritmo é o do script remoto, ou
agente antigo que não informa — fica no piso de 30 s.

## Bruto e tendência

Duas tabelas, com propósitos diferentes:

| Tabela | Granularidade | Retenção padrão |
|---|---|---|
| `metric_servers` | Uma linha por amostra (segundos) | 7 dias |
| `metric_server_trends` | Média e máximo por hora, por host | 400 dias |

Gráfico de 30 dias sobre o dado bruto varre milhões de linhas; sobre a tendência
são 24 linhas por dia por host. É o mesmo princípio dos *trends* do Zabbix: o
histórico fino tem vida curta, a série longa vive agregada.

O rollup roda a cada 15 minutos e **só olha as últimas 3 horas** na passada
incremental. Sem esse limite inferior, cada passada varria a tabela inteira e
reescrevia todos os baldes de todos os servidores — custo crescendo com o
tamanho do histórico, para reescrever dado que não mudou.

A **primeira** passada depois do boot é completa, sem a janela. Sem isso, um
painel que ficou fora do ar mais que 3 horas teria o bruto daquele período
apagado pela poda antes de virar tendência.

## Ponto ausente não é ponto zero

`GET /api/metrics/history` **omite** o ponto quando não há medição, em vez de
devolver zero. A leitura da tendência filtra com `WHERE <coluna> IS NOT NULL`.

Isso vale para a temperatura e para o handshake. Um gráfico com buraco diz "não
medi aqui"; um gráfico no chão diz "medi zero", e as duas coisas são diferentes.

## Containers

`MetricContainer` guarda CPU e memória por container, com `docker_id` como chave
do lado do Docker. O estado (`running`, `exited`) vem do `docker ps` e o consumo
do `docker stats`, unidos no Go pelo `docker_id` — os dois comandos rodam no
mesmo ciclo do script remoto.

Container só é considerado ativo dentro de uma janela fixa de 30 segundos, e não
derivada: o ritmo aqui é o do script de coleta, não o de um agente que informe o
próprio intervalo.

## Balanceador

`MetricLoadBalancer` conta requisições por `upstream_addr` a partir do access log
do Nginx, num host com `collect_nginx` ligado. A janela exibida no painel é de 5
segundos.

Desde a correção de recorte por unidade, a linha carrega `server_id` e `site_id`
preenchidos na origem — antes a tabela não tinha unidade nenhuma, e a rota de
descoberta de SSL só sabia devolver a topologia inteira ou lista vazia.

Linha gravada antes disso fica sem unidade e some sozinha em 7 dias, pela
retenção. Inventar uma unidade para linha cuja origem o sistema nunca registrou
seria adivinhação gravada como fato.
