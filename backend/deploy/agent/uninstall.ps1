# Remove o agente de estacao do vd_stats do Windows (tarefa, wrapper e
# binario).
#
# A config C:\ProgramData\vd-agent\agent.env so e apagada com confirmacao:
# ela carrega o token e o codigo da unidade, uteis numa reinstalacao.
#
# Uso (PowerShell como Administrador):
#   .\uninstall.ps1

$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "rode este script num PowerShell como Administrador"
    exit 1
}

$installDir = "C:\Program Files\vd-agent"
$dataDir    = "C:\ProgramData\vd-agent"

schtasks /end /tn vd-agent 2>$null | Out-Null
schtasks /delete /f /tn vd-agent 2>$null | Out-Null
Stop-Process -Name "vd-agent" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

if (Test-Path $installDir) {
    Remove-Item -Recurse -Force $installDir
}
Write-Host "tarefa e binario removidos"

if (Test-Path (Join-Path $dataDir "agent.env")) {
    $resposta = Read-Host "apagar tambem a config em $dataDir? [s/N]"
    if ($resposta -eq "s" -or $resposta -eq "S") {
        Remove-Item -Recurse -Force $dataDir
        Write-Host "config removida"
    } else {
        Write-Host "config preservada em $dataDir"
    }
}
