# Instalação do agente de estação

O agente (`cmd/agent`) roda em cada máquina monitorada e envia CPU, memória,
disco, temperatura, uptime e usuário logado para o painel. A máquina se
registra sozinha no primeiro envio — não é preciso cadastrá-la antes.

Este diretório contém tudo para instalar em massa: unit systemd, scripts de
instalação Linux e Windows e um playbook Ansible.

## Variáveis

Definidas em `/etc/vd-agent.env` (Linux) ou `C:\ProgramData\vd-agent\agent.env`
(Windows):

| Variável | Obrigatória | Descrição |
|---|---|---|
| `AGENT_SERVER_URL` | sim | URL do painel central, sem barra no final |
| `AGENT_TOKEN` | sim | mesmo valor de `AGENT_INGEST_TOKEN` no painel |
| `AGENT_SITE` | não | código da unidade (tela Unidades); agrupa a estação na filial certa |
| `AGENT_INTERVAL` | não | segundos entre envios (padrão 5) |
| `AGENT_HOSTNAME` | não | nome exibido no painel (padrão: hostname do sistema) |

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
sudoedit /etc/vd-agent.env              # preencha token, URL e unidade
sudo systemctl restart vd-agent
journalctl -u vd-agent -f
```

Rodar `install.sh` de novo atualiza o binário e reinicia o serviço; a config
existente nunca é sobrescrita. `uninstall.sh` remove tudo (pergunta antes de
apagar a config).

O serviço roda sem privilégio (`DynamicUser`) com hardening de systemd. O
agente só lê o sistema, então as opções `Protect*` não o atrapalham; se numa
distro específica a temperatura vier sempre 0, veja o comentário na unit.

## Windows (manual)

O binário Go ainda não implementa o protocolo de serviço do Windows, então a
instalação usa uma **tarefa agendada** que sobe o agente no boot como SYSTEM —
não um serviço de verdade. A evolução natural é o agente adotar
`golang.org/x/sys/windows/svc`; até lá, a tarefa cumpre o papel.

Num PowerShell **como Administrador**:

```powershell
# copie dist\agent-windows-amd64.exe e a pasta deploy\agent para a máquina
.\install.ps1 -SourceExe .\agent-windows-amd64.exe
notepad C:\ProgramData\vd-agent\agent.env    # preencha token, URL e unidade
schtasks /end /tn vd-agent ; schtasks /run /tn vd-agent   # relê a config
```

A config recebe ACL restrita a SYSTEM e Administradores (o token não pode
ficar legível para qualquer usuário da estação). `uninstall.ps1` desfaz tudo.

## Rollout em massa

### Ansible (Linux)

```bash
make agent-linux-amd64 agent-linux-arm64
cd deploy/agent/ansible
cp inventory.example inventory.ini            # ajuste hosts e unidades
ansible-vault create secrets.yml              # vd_agent_token: "..."
ansible-playbook -i inventory.ini vd-agent.yml --ask-vault-pass
```

O playbook escolhe o binário pela arquitetura do host, grava a config com o
token vindo do vault (nunca em texto no repositório) e só reinicia o serviço
quando algo mudou. O código da unidade é definido por grupo do inventário —
cada filial se classifica sozinha.

### GPO (Windows)

1. Compartilhe `agent-windows-amd64.exe`, `install.ps1` e um `agent.env` já
   preenchido num share acessível pelas máquinas (ex.: `\\srv\deploy\vd-agent`).
2. No GPO da OU das estações: *Computer Configuration > Policies > Windows
   Settings > Scripts > Startup*, adicione um script PowerShell chamando
   `install.ps1 -SourceExe \\srv\deploy\vd-agent\agent-windows-amd64.exe`.
3. Copie o `agent.env` preenchido para `C:\ProgramData\vd-agent\` no mesmo
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
| estação não aparece no painel | `journalctl -u vd-agent -f` — token errado dá `HTTP 401`; URL errada dá erro de conexão |
| serviço parado (Linux) | `systemctl status vd-agent`; `systemctl restart vd-agent` |
| tarefa parada (Windows) | `schtasks /query /tn vd-agent /fo LIST`; `schtasks /run /tn vd-agent` |
| temperatura sempre 0 | normal em VM/container (sem sensor); em máquina física, veja o comentário sobre `ProtectKernelTunables` na unit |
| unidade errada no painel | confira `AGENT_SITE` — o código precisa existir na tela Unidades |
| duas máquinas com o mesmo nome | defina `AGENT_HOSTNAME` distinto; o painel identifica a estação pelo hostname |
