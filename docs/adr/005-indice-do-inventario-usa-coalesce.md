# 005 — O índice do inventário usa `COALESCE(site_id, 0)`

## Contexto

`NetworkHost.IP` tinha índice único **global**. Duas filiais com faixa RFC1918
sobreposta — `192.168.0.0/24` em toda filial, que é o caso normal — colidiam na
mesma linha. Cada coletor sobrescrevia hostname, MAC e portas do outro a cada
ciclo, e o registro alternava entre dois equipamentos a cada 15 minutos.

A chave certa é `(unidade, ip)`. O problema é que `site_id` é anulável: host
descoberto antes de a unidade ser configurada não tem nenhuma.

E num índice único do Postgres, **`NULL` é distinto de `NULL`**. Com
`UNIQUE (site_id, ip)` puro, host sem unidade duplicaria a cada ciclo do coletor
— porque nenhuma das linhas jamais colidiria com a outra.

## Decisão

Índice único sobre a **expressão**:

```sql
CREATE UNIQUE INDEX idx_network_hosts_site_ip
    ON network_hosts (COALESCE(site_id, 0), ip)
```

## Consequência

**A favor.** Todos os hosts sem unidade compartilham a chave `0` e colidem entre
si como deveriam. O sentinela é seguro porque `Site.ID` é serial e começa em 1 —
nenhuma unidade real vale zero.

**Por que não `NULLS NOT DISTINCT`.** O Postgres 15 tem essa cláusula, que
resolveria o mesmo problema declarativamente. Foi descartada para não exigir 15+:
embora o `compose` fixe `postgres:15-alpine`, o projeto não declara versão mínima
em lugar nenhum, e um adotante em 14 descobriria na primeira subida.

**Contra — o custo real.** Índice sobre expressão não é expressável por tag do
GORM. Ele vive numa função de migração explícita, `migrateNetworkHostSiteIP`, que
roda depois do `AutoMigrate` e precisa remover o índice antigo pelo catálogo
(`pg_index`), porque o nome que o GORM deu varia com a versão.

**Contra — o alvo do `ON CONFLICT` precisa casar exatamente.** Alvo que não
corresponde a nenhum índice faz o Postgres recusar o `INSERT` inteiro com
`SQLSTATE 42P10`. Como há **dois** caminhos de gravação — coletor remoto e
varredura local — o alvo mora em `NetworkHostConflictTarget()`, fonte única.
Deixá-lo em cada chamador era garantia de divergirem.

**Consequência não óbvia: adoção de órfão.** A linha antiga sem unidade tem chave
`(0, ip)` e **não colide** com a nova, `(9, ip)`. Sem tratamento, o mesmo endereço
passa a existir duas vezes — uma órfã, que continua aparecendo na tela, e uma
classificada. Por isso `AdoptNetworkHostsWithoutSite` roda **antes** do upsert nos
dois caminhos.

**O mesmo padrão se repete.** `Server` usa `(COALESCE(site_id,0), machine_id)`,
com a diferença de ser índice **parcial** (`WHERE machine_id <> ''`): ali o vazio
é comum — host cadastrado a mão, agente antigo — e todos os vazios colidiriam
entre si se o índice fosse total.
