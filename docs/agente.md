# Agente de push

Binário Go instalado na máquina monitorada, que mede localmente e faz
`POST /api/ingest/metrics` a cada `AGENT_INTERVAL` segundos.

**Quando usar em vez da coleta por SSH:** máquina atrás de NAT, sem porta aberta,
ou onde abrir SSH como root não é aceitável. O agente inicia a conexão, então
basta que a máquina alcance o painel.

**O que ele não faz:** não lê containers, não abre stream de log e **não aceita
comando**. Host cadastrado como `Kind = "agent"` é explicitamente pulado pelos
coletores SSH — tentar conectar geraria alerta de host inalcançável em laço.

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
`/etc/vd-agent.env` existente **nunca é sobrescrito**, então a configuração
sobrevive à atualização.

A instalação do binário é atômica — escreve com nome temporário e renomeia — para
nunca deixar um executável pela metade se o disco encher no meio da cópia.

Desinstalar: `sudo backend/deploy/agent/uninstall.sh`.

### Windows

`install.ps1` e `uninstall.ps1` no mesmo diretório.

### Ansible

`backend/deploy/agent/ansible/`, para instalar em lote.

## Identidade por dispositivo

Este é o ponto que mais mudou, e o motivo importa.

### O problema do token compartilhado

`AGENT_INGEST_TOKEN` era **um único segredo para todos os agentes e coletores de
todas as unidades**, e a unidade do envio vinha do campo `site_code` **do corpo**,
aceito como verdade.

Consequência: uma estação comprometida em qualquer filial, de posse do token,
forjava inventário de outra unidade, criava servidor por hostname em qualquer
unidade e injetava métrica falsa que **dispara** alerta (ruído) ou **silencia**
alerta (o host parece saudável). E revogar o token derrubava o parque inteiro de
uma vez — então na prática ninguém revogava.

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

### Transição

O `AGENT_TOKEN` compartilhado **continua aceito**, com aviso no log a cada uso.
Derrubá-lo no dia do deploy silenciaria todo agente já instalado, e um painel de
monitoramento que emudece é pior que um inseguro, porque ninguém percebe.

A credencial própria **vence** o token compartilhado quando os dois estão
presentes. O contrário deixaria a migração sem efeito enquanto alguém não
limpasse os arquivos de configuração um a um.

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

Definida **na máquina monitorada**, em `/etc/vd-agent.env` no Linux.

| Variável | Padrão | Efeito |
|---|---|---|
| `AGENT_SERVER_URL` | — | URL do painel. Obrigatória |
| `AGENT_ENROLL_TOKEN` | — | Convite de uso único. Some da configuração depois do primeiro boot |
| `AGENT_TOKEN` | — | Modo compartilhado, em descontinuação |
| `AGENT_CREDENTIAL_PATH` | `/etc/vd-agent/credential.json` | Onde a credencial é gravada, modo `0600` |
| `AGENT_MACHINE_ID` | `/etc/machine-id` | Sobrescreve o identificador |
| `AGENT_HOSTNAME` | hostname do sistema | Sobrescreve o nome reportado |
| `AGENT_SITE` | — | Código da unidade. **Ignorado** quando há credencial |
| `AGENT_INTERVAL` | `5` | Segundos entre envios |

No Windows a credencial vai para `%ProgramData%\vd-agent\credential.json`.

Sem credencial gravada, sem convite e sem token compartilhado, **o agente não
sobe**. Um agente que roda sem conseguir enviar é pior que um que não roda: a
máquina some do painel sem ninguém perceber.

## O que ele reporta

CPU, memória, disco da raiz, load de 1 minuto, uptime, temperatura (quando há
sensor), sistema operacional, plataforma, arquitetura, usuário logado, versão do
agente e o próprio `AGENT_INTERVAL`.

O intervalo é reportado porque **só o agente sabe o valor real**, e o painel
deriva dele a janela de tolerância antes de dar a máquina como offline. Ver
[`metricas.md`](metricas.md).

Temperatura ausente sai **fora do JSON**, não como zero. Ver o mesmo documento.

## Diagnóstico

```bash
systemctl status vd-agent
journalctl -u vd-agent -f
```

| Mensagem no log | Significado |
|---|---|
| `credencial propria em uso (device=… unidade=…)` | Tudo certo, modo novo |
| `AVISO: usando AGENT_TOKEN compartilhado` | Modo legado; migre |
| `enrollment recusado (401)` | Convite inexistente, expirado ou já usado |
| `AVISO GRAVE: credencial obtida mas NAO gravada` | O convite foi consumido e a credencial não foi para o disco. Emita outro antes de reiniciar |
| `sem identidade` | Nenhuma das três formas de autenticação foi encontrada |

O painel responde `409` quando a unidade declarada diverge da credencial, e
registra `ingest.site_mismatch` na auditoria. É o sinal mais direto de
dispositivo comprometido que o sistema produz.
