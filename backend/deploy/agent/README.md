# Instalação do agente de estação

O agente (`cmd/agent`) roda em cada máquina monitorada e envia CPU, memória,
disco, temperatura, uptime e usuário logado para o painel. A máquina se
registra sozinha no primeiro envio — não é preciso cadastrá-la antes.

Este diretório contém tudo para instalar em massa: unit systemd, scripts de
instalação Linux e Windows e um playbook Ansible.

## Variáveis

Definidas em `/etc/dockkeeper-agent.env` (Linux) ou `C:\ProgramData\dockkeeper-agent\agent.env`
(Windows):

| Variável | Obrigatória | Descrição |
|---|---|---|
| `AGENT_SERVER_URL` | sim | URL do painel central, sem barra no final |
| `AGENT_ENROLL_TOKEN` | sim, no primeiro boot | convite de uso único emitido no painel; vira credencial própria em `/var/lib/dockkeeper-agent/credential.json` |
| `AGENT_SITE` | não | código da unidade (tela Unidades); agrupa a estação na filial certa |
| `AGENT_INTERVAL` | não | segundos entre envios (padrão 5) |
| `AGENT_HOSTNAME` | não | nome exibido no painel (padrão: hostname do sistema) |

O token compartilhado (`AGENT_TOKEN`) está descontinuado e o painel o recusa por
padrão; ver a seção "Descontinuado" de `docs/agente.md`.

## Gerar os binários

Na raiz do `backend/`:

```bash
make agent-all        # linux amd64/arm64, windows amd64, darwin arm64 -> dist/
make agent-linux-amd64   # ou um alvo específico
```

`CGO_ENABLED=0`: binário estático, roda em qualquer distro sem dependência.

## Linux (manual)

```bash
make agent-linux-amd64
sudo deploy/agent/install.sh            # usa dist/agent-linux-amd64
sudoedit /etc/dockkeeper-agent.env      # preencha convite, URL e unidade
sudo systemctl restart dockkeeper-agent
journalctl -u dockkeeper-agent -f
```

Rodar `install.sh` de novo atualiza o binário e reinicia o serviço; a config
existente nunca é sobrescrita. `uninstall.sh` remove tudo (pergunta antes de
apagar a config).

O serviço roda sem privilégio (`DynamicUser`) com hardening de systemd. O
agente só lê o sistema, então as opções `Protect*` não o atrapalham; se numa
distro específica a temperatura vier sempre 0, veja o comentário na unit.

## Windows (manual)

O agente roda como **serviço do Windows** `dockkeeper-agent`, com a conta
SYSTEM, início automático e reinício em falha depois de 30 s. Em modo serviço ele
lê a config de `C:\ProgramData\dockkeeper-agent\agent.env`. Quem instalou uma
versão anterior, que usava tarefa agendada, é migrado pelo próprio `install.ps1`.

Num PowerShell **como Administrador**:

```powershell
# copie dist\agent-windows-amd64.exe e a pasta deploy\agent para a máquina
.\install.ps1 -SourceExe .\agent-windows-amd64.exe
notepad C:\ProgramData\dockkeeper-agent\agent.env    # preencha convite, URL e unidade
Restart-Service dockkeeper-agent                          # relê a config
```

A config recebe ACL restrita a SYSTEM e Administradores (o token não pode
ficar legível para qualquer usuário da estação). `uninstall.ps1` desfaz tudo.

## Migração do nome antigo

Até setembro de 2026 o agente se chamava `vd-agent`. Os três instaladores
migram uma estação já instalada sem perder a identidade dela, e podem rodar
quantas vezes for preciso:

| O quê | Antes | Depois |
|---|---|---|
| serviço / tarefa | `vd-agent` (tarefa agendada no Windows) | `dockkeeper-agent` (serviço no Windows) |
| binário (Linux) | `/usr/local/bin/vd-agent` | `/usr/local/bin/dockkeeper-agent` |
| config (Linux) | `/etc/vd-agent.env` | `/etc/dockkeeper-agent.env` |
| credencial (Linux) | `/var/lib/vd-agent/credential.json` | `/var/lib/dockkeeper-agent/credential.json` |
| binário (Windows) | `C:\Program Files\vd-agent\` | `C:\Program Files\dockkeeper-agent\` |
| config e credencial (Windows) | `C:\ProgramData\vd-agent\` | `C:\ProgramData\dockkeeper-agent\` |
| variáveis do Ansible | `vd_agent_*` | `dockkeeper_agent_*` |

- O serviço ou a tarefa antiga é parada e removida antes da troca.
- `agent.env`, `credential.json` e `machine-id` são movidos para o lugar novo.
  A credencial e o `machine-id` preservam a identidade: sem eles a estação
  pediria convite novo e apareceria no painel como máquina nova.
- Caminhos com o nome antigo dentro da config migrada são reescritos.
- Se o destino novo já existir, ele é mantido e o arquivo antigo fica ao lado
  com o sufixo `.anterior`, para revisão.
- O playbook ainda aceita as variáveis `vd_agent_*` de inventários e vaults
  antigos, mas `dockkeeper_agent_*` vence quando as duas existem.

## Rollout em massa

### Ansible (Linux)

```bash
make agent-linux-amd64 agent-linux-arm64
cd deploy/agent/ansible
cp inventory.example inventory.ini            # ajuste hosts e unidades
ansible-vault create secrets.yml              # dockkeeper_agent_enroll_token: "..."
ansible-playbook -i inventory.ini dockkeeper-agent.yml --ask-vault-pass
```

O playbook escolhe o binário pela arquitetura do host, grava a config com o
convite vindo do vault (nunca em texto no repositório) e só reinicia o serviço
quando algo mudou. O código da unidade é definido por grupo do inventário —
cada filial se classifica sozinha.

### GPO (Windows)

1. Compartilhe `agent-windows-amd64.exe`, `install.ps1` e um `agent.env` já
   preenchido num share acessível pelas máquinas (ex.: `\\srv\deploy\dockkeeper-agent`).
2. No GPO da OU das estações: *Computer Configuration > Policies > Windows
   Settings > Scripts > Startup*, adicione um script PowerShell chamando
   `install.ps1 -SourceExe \\srv\deploy\dockkeeper-agent\agent-windows-amd64.exe`.
3. Copie o `agent.env` preenchido para `C:\ProgramData\dockkeeper-agent\` no mesmo
   script, **antes** do install (o install não sobrescreve config existente).
4. O script roda como SYSTEM no boot; a instalação é idempotente, então o GPO
   pode ficar aplicado — execuções seguintes só atualizam o binário se ele
   mudou no share.

## Atualizar a versão

- Linux manual: `make agent-linux-amd64 && sudo deploy/agent/install.sh`
- Ansible: gere os binários novos e rode o playbook de novo
- Windows/GPO: substitua o `.exe` no share; o startup script atualiza no
  próximo boot (ou rode `install.ps1` manualmente)

A versão instalada aparece na coluna "agente" da tela Estações do painel —
é por ela que se encontra máquina rodando build antiga.

## Troubleshooting

| Sintoma | Verificação |
|---|---|
| estação não aparece no painel | `journalctl -u dockkeeper-agent -f` — credencial recusada dá `HTTP 401`/`403`; URL errada dá `falha de rede` |
| serviço parado (Linux) | `systemctl status dockkeeper-agent`; `systemctl restart dockkeeper-agent` |
| serviço parado (Windows) | `Get-Service dockkeeper-agent`; `Start-Service dockkeeper-agent`; falhas aparecem no Visualizador de Eventos, em Sistema |
| temperatura sempre 0 | normal em VM/container (sem sensor); em máquina física, veja o comentário sobre `ProtectKernelTunables` na unit |
| unidade errada no painel | confira `AGENT_SITE` — o código precisa existir na tela Unidades |
| duas máquinas com o mesmo nome | o painel identifica a estação pelo `machine_id`; defina `AGENT_HOSTNAME` distinto só para diferenciar na tela |
