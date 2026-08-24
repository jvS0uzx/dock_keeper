# Instala (ou atualiza) o agente de estacao do vd_stats no Windows.
#
# O binario Go nao implementa o protocolo de servico do Windows, entao NAO
# registramos com sc.exe/New-Service (o SCM mataria o processo por nao
# responder). A instalacao usa uma TAREFA AGENDADA que sobe o agente no boot
# como SYSTEM — mesmo efeito pratico, sem o protocolo. A evolucao natural e o
# agente adotar golang.org/x/sys/windows/svc e virar servico de verdade.
#
# Uso (PowerShell como Administrador):
#   .\install.ps1 [-SourceExe caminho\agent-windows-amd64.exe]
#
# Idempotente: rodar de novo troca o binario e recria a tarefa; a config
# existente em C:\ProgramData\vd-agent\agent.env nunca e sobrescrita.

param(
    [string]$SourceExe = ".\agent-windows-amd64.exe"
)

$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "rode este script num PowerShell como Administrador"
    exit 1
}

if (-not (Test-Path $SourceExe)) {
    Write-Error "binario nao encontrado em $SourceExe (gere com: make agent-windows-amd64)"
    exit 1
}

$installDir = "C:\Program Files\vd-agent"
$dataDir    = "C:\ProgramData\vd-agent"
$exeDest    = Join-Path $installDir "vd-agent.exe"
$cmdDest    = Join-Path $installDir "vd-agent.cmd"
$envDest    = Join-Path $dataDir "agent.env"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

# Para a instancia atual antes de trocar o exe (arquivo em uso nao copia).
schtasks /end /tn vd-agent 2>$null | Out-Null
Stop-Process -Name "vd-agent" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

Copy-Item -Force $SourceExe $exeDest
Write-Host "binario instalado em $exeDest"

# Config modelo so na primeira instalacao. Valores SEM aspas e sem espacos:
# quem le e o interpretador de comandos, que nao remove aspas como o systemd.
if (-not (Test-Path $envDest)) {
    @"
# Configuracao do agente de estacao do vd_stats (Windows).
# Valores sem aspas e sem espacos.

AGENT_SERVER_URL=https://painel.exemplo.com
AGENT_TOKEN=
AGENT_SITE=
AGENT_INTERVAL=5
#AGENT_HOSTNAME=
"@ | Set-Content -Encoding ascii $envDest
    Write-Host "config modelo criada em $envDest — EDITE o token antes de usar"
} else {
    Write-Host "config existente preservada: $envDest"
}

# O token nao pode ficar legivel para qualquer usuario da estacao: corta a
# heranca de permissoes e deixa apenas SYSTEM e Administradores (SIDs S-1-5-18
# e S-1-5-32-544, imunes a idioma do Windows).
icacls $envDest /inheritance:r /grant "*S-1-5-18:F" /grant "*S-1-5-32-544:F" | Out-Null

# Wrapper: carrega o env (ignorando comentarios) e sobe o agente.
@'
@echo off
rem Carrega as variaveis de C:\ProgramData\vd-agent\agent.env e inicia o
rem agente. Gerado pelo install.ps1 — edite o .env, nao este arquivo.
for /f "usebackq eol=# tokens=1,* delims==" %%A in ("C:\ProgramData\vd-agent\agent.env") do set "%%A=%%B"
"C:\Program Files\vd-agent\vd-agent.exe"
'@ | Set-Content -Encoding ascii $cmdDest

schtasks /create /f /tn vd-agent /sc onstart /ru SYSTEM /rl HIGHEST /tr "`"$cmdDest`"" | Out-Null
schtasks /run /tn vd-agent | Out-Null

Start-Sleep -Seconds 2
schtasks /query /tn vd-agent /fo LIST | Select-String "TaskName|Status"
Write-Host ""
Write-Host "instalado como tarefa agendada 'vd-agent' (inicia no boot como SYSTEM)"
Write-Host "verificar: schtasks /query /tn vd-agent"
