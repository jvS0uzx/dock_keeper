# Operação

## Retenção e poda

Cinco prazos, todos configuráveis. Ver [`configuracao.md`](configuracao.md).

| Dado | Padrão | Racional |
|---|---|---|
| Métrica bruta | 7 dias | Inserção a cada poucos segundos por host. Depois de uma semana o que se olha é a tendência |
| Log | 7 dias | Idem |
| Tendência horária | 400 dias | Permite comparar um mês com o mesmo mês do ano anterior |
| Host do inventário | 30 dias | Cadastro, não série. Some quando o equipamento some |
| Auditoria | 365 dias | Consultada **depois** do incidente, que costuma ser descoberto meses depois |

A poda roda de hora em hora, **em lotes com pausa**. Um `DELETE` sem limite numa
tabela grande bloqueia e produz bloat; os lotes têm teto por passada, e o que
sobra fica para o ciclo seguinte, registrado no log.

### A ordem entre rollup e poda importa

`StartTrendWorker` devolve um canal que a poda espera antes da primeira passada.
Se o painel ficou fora do ar mais tempo que a janela de retenção, podar antes de
agregar apagaria dado bruto que o rollup nunca consolidou — e o histórico daquele
período sumiria para sempre.

Por isso também a **primeira** passada do rollup é completa, sem a janela de 3
horas que as demais usam.

## Auditoria

Toda escrita e todo comando remoto deixam rastro em `audit_logs`.

### O que é registrado

| Coluna | Observação |
|---|---|
| `actor_user_id`, `actor_username`, `actor_role` | Nome e papel são **copiados**, não referenciados |
| `source_ip`, `user_agent` | Origem |
| `action` | `recurso.verbo`: `container.stop`, `user.create`, `site.delete` |
| `target_type`, `target_id`, `target_label` | Rótulo copiado pelo mesmo motivo do nome |
| `site_id` | Permite recortar a auditoria por unidade |
| `result` | `ok`, `denied`, `error` ou `pending` |
| `detail` | JSON montado por allowlist, **nunca** o corpo da requisição |

O nome do ator é copiado porque usuário apagado não pode levar embora o próprio
rastro, e porque o papel muda com o tempo — o que importa é qual era **no momento
da ação**.

### O que não é registrado, e por quê

**Sucesso de ingestão.** São milhares de push por minuto num parque de algumas
centenas de hosts. Gravar cada um destruiria a tabela e afogaria o sinal. Só a
**recusa** vira linha — token inválido é exatamente o que a auditoria existe para
capturar.

**Requisições `GET`.** Leitura não é escrita. As duas exceções são a abertura dos
streams de `auth.log` e `docker logs`: leitura de dado sensível, como root, por
decisão de uma pessoa. Uma linha por abertura, nunca por evento — registrar o
conteúdo transmitido faria da tabela uma segunda cópia do log.

**O corpo da requisição.** Uma auditoria que copiasse o corpo de `POST
/api/auth/login` viraria um depósito de senha em claro na mesma tabela que o
administrador consulta. Há teste que falha se a senha aparecer em qualquer campo
da linha gravada.

### Consultar

`GET /api/audit`, admin global, paginado, filtrável por ator, ação, resultado,
unidade e intervalo. Há tela no painel.

O que procurar primeiro num incidente:

| Ação | Significa |
|---|---|
| `result = denied` | Alguém tentou alcançar unidade alheia, ou credencial recusada |
| `ingest.site_mismatch` | Dispositivo com credencial válida declarando outra unidade. **Sinal de comprometimento** |
| `device.enroll` com `denied` | Convite inexistente, expirado ou já usado |
| `auth-log.open`, `container-logs.open` | Quem leu log sensível de qual host |
| `container.*` com `pending` que nunca fechou | Comando que travou a máquina e não retornou |

## Alertas

O motor avalia as regras a cada 30 segundos sobre a última métrica de cada host.

**Duração mínima.** `for_duration_sec` exige que a condição se mantenha por N
segundos sem interrupção. Zero mantém o comportamento antigo — uma amostra acima
do limite já alerta —, que transforma qualquer pico de compilação ou backup em
incidente e ensina o operador a ignorar o canal. Uma amostra dentro do limite
**zera** a contagem.

**Cooldown persistente.** 30 minutos. Antes viviam num mapa em memória, zerado a
cada reinício: um deploy fazia o painel recomeçar notificando tudo. Desde 18/09 a
conta é feita sobre a última **entrega bem-sucedida** da chave, não sobre a última
tentativa — um envio que falhou não consome o cooldown.

**Fila de entrega (ADR 011).** Detectar e entregar deixaram de ser a mesma coisa.
`Notify` grava o alerta na tabela `alerts` (`status=open`, `delivery=pendente`) e
devolve na hora; um despachante de fundo faz o `POST` no Telegram. Consequências
no dia a dia:

- Telegram fora não perde alerta: a linha fica pendente e sai quando o canal volta.
- A entrega repete com espera dobrando de 1 min até 1 h. Depois de
  `ALERT_MAX_ATTEMPTS` (8) a entrega vira `falhou` e o alerta **continua aberto**,
  aparecendo em `GET /api/alerts?status=open` e no contador de `/api/alerts/summary`.
- O tick do motor e a reconexão SSH não esperam mais pela API do Telegram.
- Dois processos não entregam o mesmo alerta: a reserva usa `FOR UPDATE SKIP LOCKED`.
- Quem reconhece (`ack`) e quem resolve fica gravado no alerta e na auditoria.

Alerta pendente antigo é sinal de canal parado: olhe `alertas` no `/readyz` e o
`last_error` da linha.

**Notificação de recuperação.** Quando a condição deixa de ser violada e o alerta
havia sido anunciado, sai um aviso de normalização — uma vez, não a cada ciclo.

Alerta abaixo de `ALERT_MIN_SEVERITY` fica só no log e **nunca gera
recuperação**: não se anuncia o fim de um problema que nunca foi comunicado.

### Telegram

Sem `parse_mode`, de propósito. Nome de container com `_` — `nginx_proxy`, o caso
comum do Docker Compose — fazia o Telegram responder `400`, e o alerta virava só
uma linha de log. O alerta que mais importa é justamente o que nunca chega.

As severidades saem como prefixo textual: `[CRITICO]`, `[ALERTA]`, `[AVISO]`,
`[INFO]`.

#### Estado do canal, e por que ele não desliga mais sozinho

Ter `TELEGRAM_BOT_TOKEN` e `TELEGRAM_CHAT_ID` definidos significa canal **ligado**.
A verificação do boot (`getMe` e `getChat`) deixou de ser condição para ligar: se
ela falhar — a VPS reinicia com a rede ainda subindo, ou a API do Telegram está
fora naquele minuto — o canal fica `degradado`, e não desligado até o próximo
restart, que era o comportamento antigo.

O estado tem três valores:

| Estado | Significa |
|---|---|
| `desligado` | falta `TELEGRAM_BOT_TOKEN` ou `TELEGRAM_CHAT_ID`; alertas só no log |
| `ok` | última verificação ou envio deu certo |
| `degradado` | configurado, mas a última verificação ou envio falhou; o motivo vai no detalhe |

Enquanto degradado o envio **continua sendo tentado**, porque o Telegram pode ter
voltado entre um alerta e outro; um envio bem-sucedido devolve o estado para `ok`.
Em paralelo, uma rotina de fundo revalida o canal com espera crescente, de 30 s até
o teto de 5 min, e o estado aparece em `GET /readyz` (campos `alertas` e
`alertas_detalhe`). Canal degradado não derruba a prontidão do painel: quem decide
o status HTTP do `/readyz` continua sendo o banco.

## Rotação de segredo

Quatro segredos, e a ordem importa:

1. **Senha do Postgres primeiro**, com `ALTER USER`. Se o `.env` for atualizado
   antes, o painel perde o banco até o comando rodar.
2. `API_TOKEN` e `AGENT_INGEST_TOKEN`: `openssl rand -hex 32`.
3. `TELEGRAM_BOT_TOKEN`: só no BotFather, com `/revoke` e depois `/token`.
4. Atualizar `.env` e `frontend/.env`, e conferir `chmod 600` nos dois.

Reiniciar o painel **não derruba mais os logins** — as sessões vivem em
`user_sessions`. O reinício serve só para recarregar o ambiente.

`SSH_KEY_PATH` não faz parte dessa rotação: ela é a chave root única de toda a
frota, e o que ela pede é substituição por usuário dedicado, não troca de valor
— o passo a passo está na seção seguinte.

## Usuário de monitoramento com privilégio mínimo

O cadastro de servidor usa `root` como padrão (`internal/database/schema.go`),
por compatibilidade com instalações existentes — e esse padrão carrega o pior
risco do modelo agentless: **a chave de `SSH_KEY_PATH` é uma só, e comprometer
o host do painel é virar root em toda a frota de uma vez.** Um usuário dedicado
não desfaz a invasão do host do painel, mas troca "root na frota" por "um
usuário limitado na frota" e permite `PermitRootLogin no` nos hosts monitorados.

### O que o painel executa, e o que cada comando exige

| Comando remoto | Origem no código | Privilégio necessário |
|---|---|---|
| `bash -s` + `stream_metrics.sh` — lê `/proc/stat`, `/proc/meminfo`, `/proc/loadavg`, `/proc/uptime`, `/proc/net/dev`, `/sys/class/hwmon/*/temp*_input`, `df -B1 /`, `date +%s%N` | `internal/ssh/client.go` | nenhum |
| `docker ps -a --format ...` e `docker stats --no-stream --format ...` | `scripts/stream_metrics.sh` | grupo `docker` |
| `docker logs -f --tail 100 -- <nome>` | `internal/ssh/client.go` | grupo `docker` |
| `docker start\|stop\|restart -- <nome>` | `internal/ssh/actions.go` | grupo `docker` |
| `tail -n 20 -f /var/log/auth.log` | `internal/ssh/security.go` (tela de Segurança) | grupo `adm` (Debian/Ubuntu) |
| `tail -n 0 -F /var/log/auth.log` | `internal/ssh/gatilhos.go` (vigia de força bruta, sessão de fundo permanente com `AUTHLOG_WATCH=true`) | grupo `adm` (Debian/Ubuntu) |
| `tail -n 0 -F /var/log/nginx/access.log` | `scripts/stream_nginx.sh` | grupo `adm` (Debian/Ubuntu) |
| `ss -tulnp \| grep LISTEN` | `internal/ssh/security.go` | roda sem root; o **nome do processo** só aparece com privilégio |
| `keepalive@openssh.com`, requisição global na sessão de coleta já aberta, a cada `RTT_PROBE_INTERVAL` | `internal/ssh/rtt.go` | nenhum. Não abre conexão nem gera linha no `auth.log` |

### Passo a passo, no host monitorado

```bash
useradd --system --create-home --shell /bin/bash dockkeeper-monitor
usermod -aG docker,adm dockkeeper-monitor

# A chave pública do painel, com o que o painel não usa desligado.
install -d -m 700 -o dockkeeper-monitor -g dockkeeper-monitor /home/dockkeeper-monitor/.ssh
echo 'no-port-forwarding,no-agent-forwarding,no-X11-forwarding ssh-ed25519 AAAA... painel' \
  > /home/dockkeeper-monitor/.ssh/authorized_keys
chown dockkeeper-monitor:dockkeeper-monitor /home/dockkeeper-monitor/.ssh/authorized_keys
chmod 600 /home/dockkeeper-monitor/.ssh/authorized_keys

# Opcional — só quando os grupos não bastam (família RHEL) ou para validar à mão.
install -m 440 sudoers-dockkeeper-monitor.exemplo /etc/sudoers.d/dockkeeper-monitor
visudo -cf /etc/sudoers.d/dockkeeper-monitor
```

No painel, cadastre (ou edite) o servidor com `user: dockkeeper-monitor`. O padrão do
cadastro continua `root`: mudá-lo quebraria instalação existente que nunca criou
o usuário dedicado.

### `sudo` pelo painel (`SSH_USE_SUDO`)

Com `SSH_USE_SUDO=true` e usuário diferente de `root`, os quatro comandos que
pedem privilégio saem com `sudo -n` e caminho absoluto, exatamente como as
linhas ativas do `sudoers-dockkeeper-monitor.exemplo`:

| Sem `sudo` (padrão) | Com `SSH_USE_SUDO=true` |
|---|---|
| `tail -n 20 -f /var/log/auth.log` | `sudo -n /usr/bin/tail -n 20 -f /var/log/auth.log` |
| `tail -n 0 -F /var/log/auth.log` | `sudo -n /usr/bin/tail -n 0 -F /var/log/auth.log` |
| `tail -n 0 -F /var/log/nginx/access.log` | `sudo -n /usr/bin/tail -n 0 -F /var/log/nginx/access.log` |
| `ss -tulnp \| grep LISTEN` | `sudo -n /usr/bin/ss -tulnp \| grep LISTEN` |

O `-n` faz o `sudo` falhar na hora quando a regra não existe, em vez de pedir
senha numa sessão sem terminal. Por isso a variável vem desligada: em
Debian/Ubuntu os grupos `adm` e `docker` bastam, e ligar sem instalar o sudoers
esvaziaria a tela de Segurança. Os caminhos são `/usr/bin`; em RHEL com `ss` em
`/usr/sbin`, crie um link ou mantenha a variável desligada nesses hosts. Se
`SSH_AUTH_LOG_PATH` apontar para outro arquivo, a linha do sudoers tem de
acompanhar.

### Chave com passphrase e `ssh-agent`

A chave da frota pode ser protegida: `SSH_KEY_PASSPHRASE` a abre no painel. A
alternativa sem segredo em variável de ambiente é `SSH_USE_AGENT=true`, que usa
as chaves do `ssh-agent` apontado por `SSH_AUTH_SOCK`; aí o `SSH_KEY_PATH` fica
opcional. Com os dois configurados, o painel oferece primeiro a chave do arquivo
e depois as do agente.

### Limitações, com honestidade

- **Grupo `docker` equivale a root local.** Quem fala com o socket monta volume
  arbitrário e é root naquele host. O ganho do setup é o raio de explosão — a
  chave deixa de abrir root direto na frota — não isolamento dentro do host.
- **`sudo` é opcional e global.** Sem `SSH_USE_SUDO=true` os comandos viajam
  como na primeira tabela e quem sustenta o setup são os grupos. A variável vale
  para todos os servidores com usuário diferente de `root`; não há escolha por
  servidor.
- **Família RHEL:** o log é `/var/log/secure`, `600 root:root`, sem grupo `adm`.
  As saídas: `SSH_USE_SUDO=true` com a linha de `/var/log/secure` do sudoers de
  exemplo, ACL (`setfacl -m u:dockkeeper-monitor:r /var/log/secure`, reaplicada a cada
  logrotate) ou manter `root` só nesses hosts.
- **Radar de portas:** com `dockkeeper-monitor`, a coluna de processo mostra
  `System/Unknown` para processo de outro dono. Porta e protocolo continuam
  corretos.

## O que olhar quando quebra

### O painel não sobe

| Mensagem | Causa |
|---|---|
| `API_TOKEN não definido` | Falta o token. Fail-closed de propósito |
| `Configuração SSH inválida` | Falta `SSH_KNOWN_HOSTS`. Ver [`../backend/deploy/README.md`](../backend/deploy/README.md) |
| `Falha crítica ao conectar no banco` | `DATABASE_URL` errada, ou banco fora |
| `sorry, too many clients` | O teto do pool não cabe no `max_connections`. Ver `DB_MAX_OPEN_CONNS` |

### Uma tela está vazia sem erro

Esta é a classe de falha mais traiçoeira do sistema, e quase sempre é uma destas:

| Tela | Causa provável |
|---|---|
| Segurança (auth.log) | Caminho de log errado para a distribuição. Ver `SSH_AUTH_LOG_PATH` |
| Descoberta de SSL | Nenhum host com `collect_nginx` ligado — não há access log para observar |
| Inventário de rede | `DISCOVERY_CIDRS` vazio, ou painel em container sem `network_mode: host` |
| Inventário, numa unidade só | A varredura local se desligou porque a unidade tem coletor registrado. O log diz |

### Métrica parada, host "offline" reportando

Confira `report_interval_sec` do host. A janela de liveness é três vezes o
intervalo, com piso de 30 s — ver [`metricas.md`](metricas.md). Agente antigo que
não informa o intervalo fica no piso, e um agente com `AGENT_INTERVAL=60` contra
piso fixo apareceria sempre offline.

### Tela de tempo real parada atrás de proxy

`proxy_buffering off` faltando no nginx. Ele segura os eventos do SSE e a tela
fica parada **sem erro nenhum aparecer**. Ver
[`../backend/deploy/README.md`](../backend/deploy/README.md).

### Todo domínio ficou vermelho de uma vez

Esperado, se a instalação usa CA interna. A verificação de SSL passou a conferir
cadeia e hostname de verdade — antes só olhava a data de validade. Popule
`SSL_EXTRA_CA` com o bundle das CAs internas.

Certificado autoassinado continua vermelho, e corretamente: é a tela existir para
detectar isso.

## Uma instância só

O painel é feito para rodar em **uma instância**. Subir duas contra o mesmo banco
duplica o que é por processo:

- a coleta SSH roda em dobro, e cada amostra vira duas linhas;
- o motor de regras avalia duas vezes (a fila de alerta ainda desduplica pela
  reserva, mas o cooldown é contado por processo em memória);
- o ticket de SSE emitido numa instância é recusado na outra;
- o limite de login e o de ingestão contam separado em cada processo.

Para ter duas, faltaria: eleger líder com `pg_try_advisory_lock` para coleta e
workers, mover ticket e limitadores para o banco, e dar nome de instância aos
contadores. Nada disso está feito, e o compose sobe um processo só de propósito.

## Migrações do banco

O esquema é aplicado por arquivos SQL numerados, embutidos no binário
(`backend/internal/database/migracoes/`), e não mais pelo `AutoMigrate` (ADR 012).

**Conferir a versão:**

```sql
SELECT versao, nome, aplicada_em FROM schema_migrations ORDER BY versao;
SELECT tabela, motivo, linhas FROM migracao_limpeza ORDER BY id;
```

A segunda consulta mostra quantas linhas órfãs cada migração apagou; o mesmo sai
no log do boot.

**Antes do primeiro boot com o migrador, faça backup.** A migração `002` apaga
linha órfã (credencial e convite de unidade que não existe mais, concessão de
usuário apagado, estado de regra sem regra, marcador de planta sem planta e
painel sem dono):

```bash
pg_dump -U postgres dockkeeper > dockkeeper-antes-das-migracoes.sql
```

Para ver **quanto** seria apagado, sem apagar:

```sql
SELECT 'device_credentials' AS tabela, count(*) FROM device_credentials c
  WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = c.site_id)
UNION ALL SELECT 'enrollment_tokens', count(*) FROM enrollment_tokens t
  WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = t.site_id)
UNION ALL SELECT 'user_site_accesses', count(*) FROM user_site_accesses a
  WHERE (a.site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = a.site_id))
     OR NOT EXISTS (SELECT 1 FROM users u WHERE u.id = a.user_id)
UNION ALL SELECT 'alert_states', count(*) FROM alert_states st
  WHERE NOT EXISTS (SELECT 1 FROM alert_rules r WHERE r.id = st.rule_id)
UNION ALL SELECT 'floor_plan_pins', count(*) FROM floor_plan_pins p
  WHERE NOT EXISTS (SELECT 1 FROM floor_plans f WHERE f.id = p.plan_id)
UNION ALL SELECT 'dashboards', count(*) FROM dashboards d
  WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = d.owner_user_id);
```

Depois de migrar, `SELECT * FROM migracao_limpeza` mostra o que foi apagado de fato.

**Criar uma migração:**

1. Crie `backend/internal/database/migracoes/NNN_nome.sql` com o próximo número.
   O arquivo é embutido no binário, então basta compilar.
2. Escreva SQL puro. A migração roda inteira numa transação; se qualquer comando
   falhar, nada dela é aplicado.
3. Se a mudança mexe em dado (coluna nova obrigatória, enum novo), limpe ou
   normalize antes de criar a restrição, e grave a contagem em `migracao_limpeza`
   como a 002 faz.
4. Rode a suíte contra um banco vazio: `go test ./internal/database/` cria bancos
   descartáveis e aplica tudo do zero.

**Nunca edite uma migração já aplicada.** O migrador guarda o hash do arquivo e
recusa subir se ele mudar, dizendo qual versão foi editada. Corrija com uma
migração nova.

**Banco antigo, criado pelo `AutoMigrate`:** na primeira subida a `001_baseline` é
adotada sem recriar nada, e só as seguintes rodam. Não é preciso recriar o banco.

**Duas instâncias subindo juntas:** um `pg_advisory_lock` serializa; a segunda
espera e encontra tudo aplicado.

`DB_AUTOMIGRATE=true` volta ao comportamento antigo (GORM criando o esquema). Serve
para ambiente descartável; em uso normal, deixe desligado.

## Comportamento com o banco indisponível

Assimétrico de propósito, e vale conhecer antes do incidente:

| Componente | Comportamento | Por quê |
|---|---|---|
| Sessão | Falha **fechada**: recusa autenticar | Não conceder acesso sem poder verificar |
| Auditoria | Falha **aberta**: registra o erro no log e segue | O painel é a ferramenta de quem apaga incêndio |
| Cooldown de alerta | Falha **aberta**: notifica | Alerta duplicado incomoda, alerta perdido mata |
| Estado de duração de alerta | Segura o disparo de regra com duração; **nenhuma** recuperação é anunciada | Nunca tranquilizar sem evidência |

## Log e contadores

O log sai em JSON (`log/slog`), um objeto por linha, com `level`, `msg` e
`componente` (o prefixo `[Alert]`, `[RealTime]`, `[Migração]` vira campo).
`LOG_LEVEL` escolhe o mínimo: `debug`, `info` (padrão), `warn` ou `error`.

`GET /metrics` devolve os contadores do processo em texto simples:

```
dockkeeper_alertas_enfileirados 12
dockkeeper_alertas_entregues 11
dockkeeper_alertas_falhos 0
dockkeeper_logs_descartados 0
dockkeeper_migracoes_aplicadas 4
dockkeeper_panicos_recuperados 0
dockkeeper_reconexoes_ssh 3
dockkeeper_sessoes_ssh_abertas 1
```

O `/readyz` diz o que está degradado sem derrubar a prontidão: canal de alerta
degradado, alerta cuja entrega falhou, ou linha de log descartada pela fila. O
status HTTP continua dependendo só do banco, porque é ele que decide se o painel
consegue servir.

### Alerta repetido depois de um restart

A entrega é **ao menos uma vez**. Se o painel cair entre o envio aceito pelo
Telegram e a gravação do sucesso, o alerta sai de novo quando a reserva vencer.
Preferimos repetir a perder (ADR 011).

### Retenção da fila

`ALERT_RETENTION_DAYS` (padrão 90) poda alerta antigo em lote, junto com as
demais tabelas. Alerta `open` **nunca** é podado por idade: some só depois de
alguém reconhecer ou resolver.

## Limite por consulta e prazo de escrita

Toda consulta carrega `statement_timeout` (`DB_STATEMENT_TIMEOUT`, padrão 15 s),
posto no DSN: consulta travada morre sozinha em vez de segurar uma das 20
conexões do pool até o cliente desistir. As rotas de leitura passam o contexto do
pedido para o banco (`database.From(r.Context())`), então fechar a aba cancela a
consulta.

O servidor HTTP ganhou `WriteTimeout` de 60 s. Os streams de SSE **limpam esse
prazo** por conexão (`http.NewResponseController`), porque um stream de log fica
aberto por horas de propósito; o resto das rotas continua com prazo.

## Pânico numa rotina de fundo

Toda goroutine de fundo roda embrulhada pelo `internal/safego`: coleta SSH (métricas,
nginx e vigia do `auth.log`), motor de regras, escritor e poda do logstore, rollup de
tendência, retenção, vigia de SSL, varredura de rede e a revalidação do canal de alerta.

Antes, um pânico em qualquer uma delas derrubava o processo inteiro, e monitoramento e
alertas paravam juntos. Agora o pânico é recuperado, registrado com o nome da rotina e a
pilha completa (`[safego] rotina "ssh:metricas:host" entrou em pânico e será reiniciada`),
e a rotina volta sozinha, com espera de 1 s dobrando até o teto de 1 min. A espera volta
para 1 s quando a execução anterior durou pelo menos 1 min, para não punir um problema
passageiro.

O que continua encerrando a rotina, de propósito: retorno normal da função (é o caminho
do desligamento) e cancelamento do contexto.

Pânico repetido no log é sinal de defeito, não de ruído: a linha traz o nome da rotina
justamente para achar o pacote culpado.

O detalhe de cada decisão está nos [ADRs](adr/).

## Log com o banco fora do ar

O escritor do logstore grava em lote. Quando o lote falha, ele separa dois casos:

- **Falha de conexão** (o ping ao banco não volta): o lote inteiro é tentado de novo, com
  espera de 1 s dobrando até 30 s, em no máximo 6 tentativas, preservando a ordem das linhas.
  Não cai para gravação linha a linha, que com o banco fora viraria 200 INSERTs presos.
  Esgotadas as tentativas, as linhas do lote entram no contador de descarte.
- **Falha de dado** (o banco responde recusando a linha): a gravação vira linha a linha, para
  isolar a linha ruim sem perder as vizinhas.

A fila tem 10 mil linhas. Cheia, a linha nova é descartada em vez de travar o stream SSE que a
produziu. O total descartado sai em `logstore.Descartadas()`, exposto no corpo do `/readyz` como
`logs_descartados`, e o log de aviso sai no máximo uma vez por minuto.
