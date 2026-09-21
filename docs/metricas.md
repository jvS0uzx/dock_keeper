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

## CPU do host

No modo SSH a CPU sai da diferença de jiffies do `/proc/stat` entre dois ciclos.
Enquanto não há diferença — o que acontece no primeiro ciclo depois de conectar —
o script **não emite a amostra**, em vez de emitir `host_cpu: 0`. O custo é uma
amostra a menos por reconexão; o ganho é não inventar ociosidade que dispara
regra `cpu <` em falso. `DOCKKEEPER_PROC_STAT` troca o arquivo lido, e existe
para o teste.

A coluna acompanha: `MetricServer.CPUUsagePercent` e `LoadAvg1` são ponteiros, e a
amostra sem medição grava `NULL`. O live devolve `null`, o histórico pula o ponto e
a tendência ignora a amostra na média, como já acontece com temperatura, rede e RTT.

No agente, `cpu` é **opcional** no push. Um payload sem o campo é aceito e grava
`NULL`, porque a amostra ainda traz memória, disco e rede, e recusá-la inteira
perderia esses números. Zero enviado de propósito continua valendo como zero, que é
o caso da máquina ociosa: `NULL` é ausência de medição, `0` é medição de ociosidade.
Regra de alerta só avalia o que foi medido, então `cpu < 90` não dispara com `NULL`.

## Tráfego de rede

`MetricServer.NetRxBps` e `NetTxBps` são a taxa de bytes recebidos e enviados,
em **bytes por segundo**, somando as interfaces físicas da máquina.

No modo SSH o script lê `/proc/net/dev` a cada ciclo e divide a diferença pelo
tempo entre duas leituras, como faz com a CPU. A primeira amostra depois de
conectar não tem leitura anterior e sai **sem** o campo, assim como a amostra em
que algum contador voltou para trás (reinício de interface) ou em que o
`/proc/net/dev` não pôde ser lido. O relógio vem de `date +%s%N`; um `date` sem
nanossegundo (BusyBox) deixa a rede sempre ausente, nunca errada.

No agente, os dois campos são opcionais em `POST /api/ingest/metrics`. Ausente ou
negativo é gravado como `NULL`. A tendência guarda `net_rx_avg` e `net_tx_avg`,
médias que ignoram as amostras nulas.

Ficam fora da soma as interfaces cujo nome começa com `lo`, `veth`, `docker`,
`br-` ou `virbr`: loopback, pares de container e bridges. Somadas, elas contariam
o tráfego de um container duas vezes — uma na `veth`, outra na placa física. O
agente de estação usa a mesma lista.

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

Para latência de rede, use o `rtt_ms`, medido pelo prober descrito abaixo.

## RTT

`MetricServer.RTTMs` é o tempo de ida e volta de um `keepalive@openssh.com` na
conexão SSH que a coleta já mantém aberta, em milissegundos. É o número a ler como
latência de rede: sem conexão nova, sem troca de chaves, só uma requisição global
do SSH e a resposta do servidor.

A cada `RTT_PROBE_INTERVAL` (30 s) o painel manda o keepalive, com timeout de 3 s,
e guarda a última medida em memória. Cada amostra gravada pela coleta leva a
medida do momento. Sem resposta, ou sem conexão ativa, o valor é `NULL`, nunca 0.
A tendência guarda `rtt_avg` e `rtt_max`. Servidor que só reporta por agente de push
fica sem RTT: o painel não tem conexão SSH com ele.

**O keepalive também detecta conexão meio-aberta.** Quando um NAT ou firewall
derruba a conexão em silêncio, o stream de métricas ficava parado esperando dado
até o timeout do TCP, que pode levar muitos minutos. Agora, depois de
`SSH_KEEPALIVE_MAX_MISSES` (3) keepalives seguidos sem resposta, o painel fecha a
conexão, registra no log que a tratou como morta, e a reconexão segue o backoff
normal.

`RTT_PROBE=false` desliga só a gravação do RTT. O keepalive continua rodando,
porque a detecção de conexão morta não depende dele.

Não há linha no `auth.log` do host: o keepalive viaja dentro da sessão já
autenticada.

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

A busca no banco segue a mesma regra. Painel e motor leem a última amostra de cada host pela
mesma consulta, `database.ConsultaUltimasMetricas`, que olha para trás exatamente a janela do
host (`GREATEST(report_interval_sec * 3, 30)` segundos). Antes havia um `metricLookback` de
10 minutos cravado nos dois lugares, abaixo da janela de qualquer agente com intervalo acima de
200 s: um agente de 5 min tinha janela de 15 min e nunca era encontrado.

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

O histórico lê a tendência em toda janela acima de 24 h, fixa ou customizada: um
ponto por hora até 30 dias, a média de 6 horas até 90 dias e a média do dia acima
disso, até o teto de 400 dias da retenção padrão. Os parâmetros estão em
[`api.md`](api.md).

A **primeira** passada depois do boot é completa, sem a janela. Sem isso, um
painel que ficou fora do ar mais que 3 horas teria o bruto daquele período
apagado pela poda antes de virar tendência.

## Ponto ausente não é ponto zero

`GET /api/metrics/history` **omite** o ponto quando não há medição, em vez de
devolver zero. A leitura da tendência filtra com `WHERE <coluna> IS NOT NULL`.

Isso vale para a temperatura, o tráfego de rede e o handshake. Um gráfico com buraco diz "não
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

O status gravado é um balde: os códigos acompanhados (500, 502, 503, 504, 429,
404, 400) ou `200` para qualquer outro. O código real é o primeiro campo de três
dígitos depois do método e do caminho — antes o parser procurava o código em
qualquer ponto da linha e classificava `GET /x 200 404` como 404, confundindo o
tamanho da resposta com o status.

## Gatilhos de alerta sem regra

Além do motor de regras, os avisos abaixo saem sozinhos, com o mesmo cooldown por
chave (`ALERT_COOLDOWN`) e o mesmo piso (`ALERT_MIN_SEVERITY`). Todos carregam a origem
(`server_id` e `site_id`), então o operador da filial enxerga o alerta do próprio servidor.

| Aviso | Chave | Quando |
|---|---|---|
| `[CRITICO]` stream do nginx caiu | `nginx_down:<servidor>` | A sessão que lê o access log cai, em servidor com `collect_nginx` |
| `[ALERTA]` upstream com 5xx | `lb_upstream_5xx:<servidor>:<upstream>` | Na janela `LB_WINDOW` (5 min), o upstream recebeu pelo menos `LB_MIN_REQUESTS` (20) requisições e a proporção de 5xx chegou a `LB_ERROR_RATIO` (0,5) |
| `[ALERTA]` força bruta | `bruteforce:<servidor>:<ip>` | Um mesmo IP de origem acumulou `BRUTEFORCE_THRESHOLD` (10) ou mais tentativas falhas na janela `BRUTEFORCE_WINDOW` (5 min) |
| `[CRITICO]` VPS inalcançável | `host_unreachable:<servidor>` | O stream de métricas por SSH cai |
| `[ALERTA]` container parado | `container_down:<servidor>:<container>` | O `docker ps` do host devolve o container em estado diferente de `running` |
| `[CRITICO]` certificado inválido | `ssl_invalid:<domínio>` | A verificação TLS do domínio falha |
| `[ALERTA]` certificado vencendo | `ssl_expiring:<domínio>` | Certificado válido com 14 dias ou menos |
| `[ALERTA]` estação sem reportar | `agent_absent:<servidor>` | Servidor `kind=agent` **marcado com `absence_alert`** sem métrica há mais de 3 vezes o `report_interval_sec` (piso de 30 s) |
| `[CRITICO]` coletor sem reportar | `collector_absent:<device_id>` | Credencial `collector` sem contato há mais de 3 vezes o intervalo de inventário (declarado, ou 15 min) |

### Quando cada gatilho se resolve sozinho

Todo gatilho fecha o próprio alerta. "Voltou" tem definição por gatilho, e sempre exige
evidência positiva: silêncio não resolve nada.

| Gatilho | O alerta vira `resolved` quando |
|---|---|
| `host_unreachable` | O SSH reconectou **e** a primeira amostra de métrica da nova sessão foi lida. Conectar e cair antes da amostra não conta |
| `nginx_down` | A sessão que lê o access log abriu e continua de pé 30 s depois. Sessão que abre e cai em seguida (arquivo inexistente, permissão) não conta |
| `container_down` | O mesmo container aparece como `running`. Container removido do host **não** resolve: sumir não é voltar, e o alerta fica para o operador fechar |
| `ssl_invalid` | A verificação seguinte encontra o certificado válido |
| `ssl_expiring` | O certificado está válido com mais de 14 dias (foi renovado) |
| `bruteforce` | A janela `BRUTEFORCE_WINDOW` esvaziou: nenhuma falha de login daquele IP no período. Conferido a cada fatia da janela, não só quando chega linha nova |
| `lb_upstream_5xx` | Na janela `LB_WINDOW` o upstream recebeu pelo menos `LB_MIN_REQUESTS` e a proporção de 5xx ficou abaixo de `LB_ERROR_RATIO`. Upstream que parou de receber tráfego **não** resolve: o balanceador pode tê-lo tirado de rotação justamente por estar morto |
| `agent_absent`, `collector_absent` | O dispositivo voltou a reportar dentro da janela. Dispositivo revogado ou servidor removido fecha o alerta em silêncio, sem mensagem de recuperação |
| regra do motor | A métrica voltou para dentro do limite |

O painel lembra o que estava aberto: ao reiniciar, cada vigia recarrega do banco os alertas
abertos do seu servidor, então um container que voltou a rodar durante a parada do painel
ainda fecha o alerta dele.

### Alerta de ausência

`host_unreachable` só existe para servidor coletado por SSH. Para a estação com agente e para o
coletor, quem percebe o silêncio é a vigia de ausência (`ABSENCE_ALERT`, ligada por padrão), que
roda a cada 30 s:

- **Agente, só o marcado:** estação desligada às 18h é rotina, e alerta que dispara todo dia por
  motivo normal mata o sistema de alerta. Por isso a estação só é vigiada quando o servidor tem
  `absence_alert` verdadeiro (`PATCH /api/servers?id=` com `{"absence_alert":true}`; padrão
  `false`), e o aviso sai como `[ALERTA]`, não `[CRITICO]`. Desmarcar fecha o alerta aberto em
  silêncio. A conta é a última métrica gravada contra `LiveWindowFor(report_interval_sec)`, isto é, 3 vezes
  o intervalo declarado, com piso de 30 s.
- **Coletor:** `last_seen_at` da credencial contra 3 vezes o intervalo de inventário. O coletor
  declara o intervalo em `report_interval_sec` no envio; sem isso o painel assume 15 min (janela
  de 45 min). Coletor configurado com intervalo maior **precisa** declarar, ou alerta em falso.
  Máquina reinstalada (mesmo `machine_id`, credencial nova) conta pela credencial vista por último.
- **Quem não alerta:** credencial revogada, servidor removido, e coletor autenticado só pelo
  token compartilhado legado, porque não há credencial para acompanhar. Agente legado é
  acompanhado normalmente, pela métrica.
- **Painel recém-iniciado:** nenhum dispositivo é acusado antes de o painel estar no ar há uma
  janela inteira. Enquanto ele esteve fora ninguém conseguia reportar.

Coletor é infraestrutura: alerta sempre, como `[CRITICO]`. Marque como vigiada a estação que
precisa ficar ligada (recepção, servidor de arquivos com agente); não há horário de silêncio por
unidade.

O vigia de força bruta é uma sessão SSH de fundo por servidor, ligada por padrão
(`AUTHLOG_WATCH`). Ele roda `tail -n 0 -F` no `auth.log` — com `sudo` quando
`SSH_USE_SUDO` pede — e só **conta** as linhas: nada vai para o banco de logs.
O `-F` segue o nome do arquivo, então a vigilância sobrevive ao logrotate; o
`-n 0` começa do fim, então reconectar não reconta falhas antigas. Como é sessão de
fundo, não entra no limite `SSH_MAX_SESSIONS_PER_HOST`.

Uma tentativa conta uma vez. Contam as linhas `Failed password` e `Invalid user`,
mas não `Failed password for invalid user`: o sshd registra a tentativa com
usuário inexistente nas duas formas, e ela já entrou pela linha `Invalid user`.

Motor de regras: além de `cpu`, `mem`, `disk` e `load`, aceita `temperature`,
`net_rx`, `net_tx` e `rtt`. Amostra sem a medição (`NULL`) é ignorada: não dispara nem
conta como zero, e também não encerra um alerta aberto.

## Endereços declarados

O script de coleta emite `addresses` com os IPv4 de todas as interfaces do host, menos
loopback e as virtuais do Docker (`veth`, `docker`, `br-`, `virbr`). Sem o comando `ip` no
host, o campo sai vazio e nada quebra. O agente de estação faz o mesmo pela gopsutil.

O painel guarda cada endereço em `server_addresses` com `primeiro_visto` e `ultimo_visto`, o
que permite duas coisas: rotular um upstream do nginx pelo servidor dono do endereço, sem
adivinhar pelo último octeto, e recusar o cadastro de um servidor com endereço que já é de
outro. Endereço coletado que passa `ADDRESS_RETENTION_DAYS` (padrão 30) sem aparecer é podado
junto com as outras retenções; alias manual fica.

## Membro do balanceador

A malha do painel desenhava só quem tinha upstream na janela do minuto, então a VPS sumia do
desenho quando o tráfego zerava. Agora a participação vem de uma janela de
`LB_MEMBERSHIP_DAYS` (padrão 7) sobre `metric_load_balancers`, cruzada com os endereços
conhecidos do servidor, e pode ser fixada à mão em `servers.behind_lb`. O `GET
/api/metrics/live` entrega o resultado já resolvido em `behind_lb`, com a procedência em
`behind_lb_origem`.
