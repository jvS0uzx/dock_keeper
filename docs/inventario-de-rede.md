# Inventário de rede

Duas fontes alimentam a mesma tabela `network_hosts`: a **varredura local**, dentro
do painel, e o **coletor remoto** (`vd_collector`), instalado numa máquina de cada
filial.

Elas existem porque o painel só enxerga a rede onde ele roda. Uma empresa com dez
filiais precisaria de dez painéis — ou de um coletor por filial.

## A chave é `(unidade, ip)`

Este é o ponto que mais custou a acertar.

`NetworkHost.IP` tinha índice único **global**. Duas filiais com faixa RFC1918
sobreposta — `192.168.0.0/24` em toda filial, que é o caso normal, não o
excepcional — colidiam na mesma linha. Cada coletor sobrescrevia hostname, MAC e
portas do outro a cada ciclo, e o mesmo registro alternava entre dois
equipamentos diferentes a cada 15 minutos.

Hoje a unicidade é composta, num índice de **expressão**:

```sql
CREATE UNIQUE INDEX idx_network_hosts_site_ip
    ON network_hosts (COALESCE(site_id, 0), ip)
```

`COALESCE(site_id, 0)` e não `(site_id, ip)` puro porque o Postgres trata `NULL`
como distinto de `NULL` num índice único: host ainda sem unidade duplicaria a cada
ciclo. O sentinela `0` é seguro porque `Site.ID` é serial e começa em 1.

Foi preferido a `NULLS NOT DISTINCT` para não exigir PostgreSQL 15+ — embora o
`compose` fixe `postgres:15-alpine`, o projeto não declara versão mínima em
lugar nenhum.

Tag de GORM não escreve índice sobre expressão, então isso vive numa função de
migração explícita em `internal/database/connection.go`.

### Adoção de host sem unidade

Consequência direta e não óbvia da chave composta: a linha que uma varredura
antiga deixou **sem unidade** tem chave `(0, ip)` e **não colide** com a linha que
a varredura seguinte grava já sabendo a unidade, `(9, ip)`.

Sem tratamento, o mesmo endereço passaria a existir duas vezes — uma órfã, que
continua aparecendo na tela, e uma classificada.

`AdoptNetworkHostsWithoutSite` roda **antes** do upsert nos dois caminhos de
gravação e entrega as linhas órfãs à unidade, respeitando `site_locked` e usando
`NOT EXISTS` para não violar o índice quando as duas linhas já existem.

## As travas do operador

Dois campos protegem a correção manual de ser desfeita pela próxima varredura:

| Campo | O que congela |
|---|---|
| `device_type_locked` | O tipo do equipamento |
| `site_locked` | A unidade |

Sem `device_type_locked`, a varredura reinferia o tipo a cada ciclo e desfazia a
correção do técnico em no máximo 15 minutos — corrigir "Desconhecido" para
"Impressora" durava um ciclo.

Sem `site_locked` era pior: o coletor **sempre** envia a unidade dele, então o
`COALESCE` sozinho nunca era nulo e revertia todo host que o operador tivesse
movido. A trava é o que faz a escolha manual vencer.

Os campos cadastrais — sala, dono, patrimônio, andar, setor, rack — **nunca** são
tocados por nenhuma varredura.

## Classificação de equipamento

O tipo é inferido pelas portas abertas, e **a ordem importa**: a primeira porta que
casar decide, então o sinal mais específico vem antes.

| Porta | Tipo | Por quê |
|---|---|---|
| 9100 | Impressora | Fila de impressão bruta identifica melhor que 80, que qualquer coisa com interface web abre |
| 515, 631 | Impressora | LPD e IPP |
| 5000 | NAS | Antes das portas Windows: um NAS também publica SMB (445), então o SMB sozinho não distingue NAS de estação |
| 3389, 445, 139, 135 | Windows | |
| 22 | Linux | |
| 443, 80 | Dispositivo web | Último recurso |

### As listas precisam concordar

A lista **sondada** (`DefaultPorts`) tem que conter toda porta que a tabela de
classificação consulta. Ela não continha 515, 631 nem 5000: a varredura local
nunca sondava o que a própria tabela consultava, então impressora que só publica
631 caía como "dispositivo web", e NAS virava estação Windows por causa do 445.

O coletor remoto já sondava as doze. O sintoma era o mesmo equipamento **mudando
de tipo conforme quem o encontrasse**. Há um teste que trava essa concordância.

## Dois escritores na mesma unidade se atrapalham

Varredura local ligada numa unidade que já tem coletor remoto registrado são dois
escritores no mesmo inventário. Cada ciclo sobrescreve o do outro, e qualquer
diferença de configuração entre eles faz o registro oscilar sozinho.

Por isso a varredura local **se desliga** quando encontra um `DeviceCredential`
com `kind = collector` e `revoked_at IS NULL` para a unidade dela:

```
[Discovery] a unidade "filial-a" já tem 1 coletor(es) registrado(s):
varredura local DESLIGADA para os dois não disputarem o inventário.
Remova DISCOVERY_CIDRS, ou revogue o coletor se quiser varrer daqui.
```

O coletor vence porque enxerga a rede da filial; o painel só enxerga a dele.

A conferência acontece na resolução da unidade, e não na leitura da configuração,
porque depende do banco — que ainda não subiu naquele ponto do boot. Coletor
**revogado** não conta: revogar um dispositivo não pode deixar a unidade sem
nenhuma fonte de inventário.

## Limites da varredura local

| Limite | Valor | Por quê |
|---|---|---|
| Faixas aceitas | Só RFC1918 | O recurso existe para inventariar a rede da própria seção, não redes de terceiros |
| Prefixo mínimo | `/16` | Maior que isso são 65 mil hosts: varredura longa demais, e provavelmente erro de digitação |
| Timeout por porta | 400 ms | |
| Concorrência | 256 sondas | |

Unidade configurada em `DISCOVERY_SITE` mas ausente do banco **não derruba** a
varredura: o inventário continua sendo gravado sem classificação, que é melhor
que perder a coleta inteira por um código digitado errado.

### `/proc/net/arp` em container

A resolução de MAC lê `/proc/net/arp`. **Dentro de um container isso é a tabela ARP
do namespace, não a do host**, e a varredura enxerga quase nada.

Quem depende da varredura local precisa de `network_mode: host` — o que descarta
a rede do compose e o nome `postgres` como host do banco — ou do coletor remoto
rodando fora do container. O `docker-compose.yml` traz essa decisão comentada.

## Poda

Host sem ser visto por `HOST_RETENTION_DAYS` (padrão 30) sai do inventário. Curto
demais apaga notebook de quem tirou férias; longo demais deixa a tela cheia de
máquina que não existe mais.

## Planta baixa

Os marcadores da planta identificam o host por **IP mais a unidade da planta** —
que é a mesma chave do inventário. `FloorPlan.SiteID` já existe e
`createFloorPlan` recusa planta sem unidade, então o par está completo sem coluna
nova.

Antes disso o marcador resolvia só por IP, e com o mesmo endereço podendo existir
em duas unidades ele podia exibir o estado do equipamento da filial errada.

Marcador apontando para host **fora do inventário** continua sendo aceito: é
estado projetado, não erro — o operador posiciona a máquina na planta antes de a
varredura chegar nela, e a tela escreve "fora do inventário". Só endereço
malformado é recusado com `400`.
