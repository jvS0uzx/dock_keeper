# Agente de push

Binário Go instalado na máquina monitorada, que mede localmente e faz
`POST /api/ingest/metrics` a cada `AGENT_INTERVAL` segundos.

**Quando usar em vez da coleta por SSH:** máquina atrás de NAT, sem porta aberta,
ou onde abrir SSH como root não é aceitável. O agente inicia a conexão, então
basta que a máquina alcance o painel.

**O que ele não faz:** não lê containers, não abre stream de log e **não aceita
comando**. Host cadastrado como `Kind = "agent"` é explicitamente pulado pelos
coletores SSH — tentar conectar geraria alerta de host inalcançável em laço.

## Transporte

`AGENT_SERVER_URL` precisa ser `https://` quando o painel não está na própria máquina. Com
`http://` para um host remoto o agente **recusa subir**, porque a credencial do dispositivo
viajaria em claro na rede. Exceções:

- `http://localhost`, `http://127.0.0.1` e `http://[::1]` continuam valendo, para
  desenvolvimento;
- `ALLOW_INSECURE_HTTP=true` libera o `http://` remoto, e o agente registra um aviso a cada
  subida dizendo que a credencial viaja em claro.

## Conta do serviço no Windows

O instalador cria o serviço e depois o move para a **conta virtual** `NT SERVICE\dockkeeper-agent`
(`sc.exe config ... obj=`), em vez de deixá-lo como SYSTEM. A conta virtual existe só para este
serviço, não tem senha e não serve para logon. O instalador dá a ela escrita em
`C:\ProgramData\dockkeeper-agent` (onde ficam `agent.env`, `credential.json` e `machine-id`) e
leitura do `agent.env`.

Se a troca de conta falhar, o instalador avisa e o serviço continua subindo como SYSTEM, para não
deixar a estação sem monitoramento; nesse caso vale investigar antes de aceitar o privilégio
extra. Esta parte **não foi executada em Windows**: não há PowerShell nesta máquina de
desenvolvimento.

## Compilar

```bash
cd backend
make agent-all              # linux amd64/arm64, windows amd64, darwin arm64
make agent-linux-amd64      # só um alvo
```

`CGO_ENABLED=0` gera binário estático, sem dependência da libc do destino — o que
importa quando as estações são heterogêneas. `-s -w` tira símbolos e DWARF.

## Instalar

### Linux (systemd)

```bash
sudo backend/deploy/agent/install.sh dist/agent-linux-amd64
```

É idempotente: rodar de novo troca o binário e reinicia o serviço. O
`/etc/dockkeeper-agent.env` existente **nunca é sobrescrito**, então a configuração
sobrevive à atualização.

A instalação do binário é atômica — escreve com nome temporário e renomeia — para
nunca deixar um executável pela metade se o disco encher no meio da cópia.

Desinstalar: `sudo backend/deploy/agent/uninstall.sh`.

Estação instalada com o nome antigo do agente é migrada pelo próprio
`install.sh` (e pelo `install.ps1` e pelo playbook): serviço, config,
credencial e `machine-id` passam para o nome `dockkeeper-agent` sem perder a
identidade. O mapa completo está em `backend/deploy/agent/README.md`, seção
"Migração do nome antigo".

### Windows

`install.ps1` e `uninstall.ps1` no mesmo diretório, num PowerShell como
Administrador:

```powershell
.\install.ps1 -SourceExe .\agent-windows-amd64.exe
```

O instalador cria `C:\ProgramData\dockkeeper-agent\agent.env` só na primeira vez, já com
a linha `AGENT_ENROLL_TOKEN=`. O fluxo de enroll é o mesmo do Linux:

1. no painel, um admin emite um convite do tipo `agent` para a unidade da estação;
2. cole o convite em `AGENT_ENROLL_TOKEN=` e preencha `AGENT_SERVER_URL`;
3. rode `Restart-Service dockkeeper-agent`. Na subida o agente troca o convite
   pela credencial própria e a grava em
   `C:\ProgramData\dockkeeper-agent\credential.json`;
4. apague o valor de `AGENT_ENROLL_TOKEN`. O convite é de uso único e já foi
   consumido; a credencial gravada vence em todo boot seguinte.

O diretório `C:\ProgramData\dockkeeper-agent` perde a herança de permissões e fica
acessível só a `SYSTEM` e Administradores, porque guarda o `agent.env` e a
credencial.

O agente roda como **serviço do Windows** (`dockkeeper-agent`, conta `SYSTEM`,
início automático). O binário implementa o protocolo de serviço
(`golang.org/x/sys/windows/svc`): `Stop-Service` e o desligamento da máquina
encerram o laço sem deixar envio pela metade. Em modo serviço ele lê a
configuração de `C:\ProgramData\dockkeeper-agent\agent.env`, ou do arquivo
apontado por `AGENT_ENV_FILE`. Linhas vazias e começadas por `#` são ignoradas, e
aspas em volta do valor são removidas. Se o processo cair, o Windows o reinicia
depois de 30 s.

O instalador migra quem vinha da versão com tarefa agendada: apaga as tarefas
`dockkeeper-agent` e `vd-agent` e o wrapper `.cmd`, e cria o serviço no lugar.

### Ansible

`backend/deploy/agent/ansible/`, para instalar em lote.

## Identidade por dispositivo

Cada agente tem credencial própria, amarrada a uma unidade. Ela nasce de um
convite de uso único, emitido na tela **Dispositivos** do painel ou por
`POST /api/enroll/tokens`.

### O fluxo de enrollment

```
 admin                  painel                      agente
   │                      │                           │
   │ POST /api/enroll/tokens                          │
   │  {site_id, kind}     │                           │
   │─────────────────────►│                           │
   │◄─────────────────────│ convite em claro,         │
   │  (sai UMA vez)       │ válido 24 h, uso único    │
   │                      │                           │
   │  entrega fora de banda ──────────────────────────►│
   │                      │                           │
   │                      │◄──────────────────────────│ POST /api/enroll
   │                      │   {token, machine_id,     │
   │                      │    hostname, kind}        │
   │                      │                           │
   │                      │──────────────────────────►│ 201 {device_id,
   │                      │  queima o convite na       │      device_token}
   │                      │  MESMA transação           │  grava 0600 em disco
   │                      │                           │
   │                      │◄──────────────────────────│ POST /api/ingest/metrics
   │                      │   X-Device-Id / X-Device-Token
```

O que cada passo garante:

**A unidade sai do convite, não do pedido.** Quem se cadastra não escolhe a que
unidade pertence.

**Queima e criação na mesma transação.** Fora dela, dois instaladores concorrentes
usam o mesmo convite duas vezes e "uso único" vira promessa.

**O segredo sai uma vez.** O banco guarda só o hash; não existe rota que releia o
valor. Perdeu, emite outro.

**A unidade do envio passa a vir da credencial.** O `site_code` do corpo, se vier,
é **conferido** — divergência responde `409`, descarta o envio inteiro e grava
uma linha de auditoria `ingest.site_mismatch`. Aceitar parcialmente seria aceitar
a parte que o atacante escolheu.

**Revogar é marcar, não apagar.** `DELETE /api/devices?device_id=…` grava
`revoked_at`. A linha fica, porque o rastro de auditoria precisa continuar
apontando para um dispositivo que existiu. Revogar um dispositivo **não afeta
nenhum outro** — que era o objetivo.

## Identificador de máquina

`machine_id` é a chave estável do host, preferida ao hostname — que muda quando
alguém renomeia a estação, partindo o histórico da mesma máquina em duas séries.

Ordem de resolução:

1. `AGENT_MACHINE_ID`, se definida;
2. `/etc/machine-id`, padrão em Linux com systemd;
3. `/var/lib/dbus/machine-id`, para as distribuições que não criam o primeiro;
4. um identificador gerado e persistido ao lado da credencial.

O quarto caso vale menos que os anteriores — reinstalar o agente gera outro — mas
vale mais que hostname.

No painel, a chave do servidor é `(unidade, machine_id)` quando o identificador
existe, com fallback para `(unidade, hostname)` em agente antigo. O índice único
é **parcial** (`WHERE machine_id <> ''`), senão todos os vazios colidiriam entre
si.

## Configuração

Definida **na máquina monitorada**, em `/etc/dockkeeper-agent.env` no Linux.

| Variável | Padrão | Efeito |
|---|---|---|
| `AGENT_SERVER_URL` | — | URL do painel. Obrigatória |
| `AGENT_ENROLL_TOKEN` | — | Convite de uso único. Some da configuração depois do primeiro boot |
| `AGENT_CREDENTIAL_PATH` | `/var/lib/dockkeeper-agent/credential.json` | Onde a credencial é gravada, modo `0600` |
| `AGENT_MACHINE_ID` | `/etc/machine-id` | Sobrescreve o identificador |
| `AGENT_HOSTNAME` | hostname do sistema | Sobrescreve o nome reportado |
| `AGENT_SITE` | — | Código da unidade. **Ignorado** quando há credencial |
| `AGENT_INTERVAL` | `5` | Segundos entre envios |

No Windows a credencial vai para `%ProgramData%\dockkeeper-agent\credential.json`.

Sem credencial gravada e sem convite, **o agente não sobe**. Um agente que roda sem conseguir enviar é pior que um que não roda: a
máquina some do painel sem ninguém perceber.

## O que ele reporta

CPU, memória, disco da raiz, load de 1 minuto, uptime, temperatura (quando há
sensor), taxa de rede recebida e enviada, sistema operacional, plataforma,
arquitetura, usuário logado, versão do agente e o próprio `AGENT_INTERVAL`.

O intervalo é reportado porque **só o agente sabe o valor real**, e o painel
deriva dele a janela de tolerância antes de dar a máquina como offline. Ver
[`metricas.md`](metricas.md).

Temperatura ausente sai **fora do JSON**, não como zero. Ver o mesmo documento.

O mesmo vale para `cpu` e `load1`, desde 19/09/2026. Se a leitura falha, o campo não sai
do envio e o painel grava `NULL`; o resto da amostra (memória, disco, rede) segue
normalmente. **No Windows `load1` nunca é enviado**, porque a plataforma não tem load
average: antes o agente mandava `0.00` como se fosse medida, e uma regra `load < X`
ou `cpu < X` disparava em falso em toda estação. Um zero medido de verdade (máquina
ociosa) continua saindo como `0`. No log de envio, a CPU sem leitura aparece como
`cpu=sem medida`.

A taxa de rede vai em `net_rx_bps` e `net_tx_bps`, em bytes por segundo. O agente
soma os contadores das interfaces físicas e divide a diferença entre dois ciclos
pelo tempo decorrido. Ficam de fora as interfaces cujo nome começa com `lo`,
`veth`, `docker`, `br-` ou `virbr`, a mesma lista da coleta por SSH, e a
`Loopback Pseudo-Interface` do Windows: bridge e `veth` do Docker repetem o
tráfego dos containers, que já passou pela interface física, e somá-las contaria
o mesmo byte duas vezes. Por isso:

- o **primeiro envio** depois do boot do agente não traz os dois campos, porque
  ainda não há leitura anterior;
- um contador que **volta** (interface recriada, contador zerado) não gera taxa
  negativa: aquele ciclo sai sem o campo e o seguinte já usa a leitura nova;
- falha ao ler os contadores também tira os campos do envio, sem derrubar o
  resto da amostra.

Como a temperatura, taxa ausente é ausência de medição, não zero: o painel a
mostra como "—".

## Diagnóstico

```bash
systemctl status dockkeeper-agent
journalctl -u dockkeeper-agent -f
```

| Mensagem no log | Significado |
|---|---|
| `credencial propria em uso (device=… unidade=…)` | Tudo certo, modo novo |
| `enrollment recusado (401)` | Convite inexistente, expirado ou já usado |
| `AVISO GRAVE: credencial obtida mas NAO gravada` | O convite foi consumido e a credencial não foi para o disco. Emita outro antes de reiniciar |
| `sem identidade` | Não há credencial gravada nem convite em `AGENT_ENROLL_TOKEN` |
| `credencial recusada pelo painel (HTTP 401)` ou `(HTTP 403)` | O painel recusou a credencial do envio: dispositivo revogado ou credencial de outro tipo. O agente continua vivo e tenta a cada ciclo; emita um novo convite |
| `falha de rede: …` | O painel não foi alcançado (DNS, conexão recusada, timeout). Não é problema de credencial |
| `painel respondeu HTTP …` | O painel recebeu o envio e devolveu outro erro; veja o log do painel |
| `encerrado` | O agente recebeu SIGTERM ou SIGINT (`systemctl stop`) e saiu do laço sem deixar envio pela metade |

O painel responde `409` quando a unidade declarada diverge da credencial, e
registra `ingest.site_mismatch` na auditoria. É o sinal mais direto de
dispositivo comprometido que o sistema produz.

## Descontinuado: token compartilhado

Antes da credencial por dispositivo, todo agente e todo coletor usava o mesmo
segredo: `AGENT_TOKEN` na estação, igual ao `AGENT_INGEST_TOKEN` do painel. A
unidade do envio vinha do campo `site_code` do corpo, aceito como verdade. Uma
estação comprometida em qualquer filial forjava inventário de outra unidade e
injetava métrica que dispara ou silencia alerta, e revogar o token derrubava o
parque inteiro de uma vez.

O painel **recusa esse token por padrão**: o envio volta `401`. Durante a
migração de um parque antigo, o painel aceita o token compartilhado só com
`ALLOW_LEGACY_INGEST_TOKEN=true` no `.env` dele, e registra aviso a cada uso.

No agente, `AGENT_TOKEN` ainda funciona quando não há credencial gravada nem
convite. A credencial própria vence o token quando os dois existem. Nesse modo o
log mostra:

| Mensagem no log | Significado |
|---|---|
| `AVISO: usando AGENT_TOKEN compartilhado, descontinuado` | Modo legado ativo na estação |
| `credencial recusada pelo painel (HTTP 401); o token compartilhado so e aceito com ALLOW_LEGACY_INGEST_TOKEN=true` | O painel não está com a flag ligada |

Para migrar uma estação: emita um convite do tipo agente para a unidade dela,
coloque-o em `AGENT_ENROLL_TOKEN`, apague `AGENT_TOKEN` e reinicie o serviço. No
primeiro envio o agente grava a credencial própria e deixa de depender da flag.
