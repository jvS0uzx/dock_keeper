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

$nome       = "dockkeeper-agent"
$antigo     = "vd-agent"
$installDir = "C:\Program Files\$nome"
$dataDir    = "C:\ProgramData\$nome"
$exeDest    = Join-Path $installDir "$nome.exe"
$cmdDest    = Join-Path $installDir "$nome.cmd"
$envDest    = Join-Path $dataDir "agent.env"
$installAntigo = "C:\Program Files\$antigo"
$dataAntigo    = "C:\ProgramData\$antigo"

function Invoke-Silencioso([string]$linha) {
    cmd.exe /c "$linha >nul 2>&1" | Out-Null
}

function Move-SemSobrescrever([string]$origem, [string]$destino) {
    if (-not (Test-Path $destino)) {
        Move-Item -LiteralPath $origem -Destination $destino
        Write-Host "  migrado: $origem -> $destino"
    } else {
        Move-Item -Force -LiteralPath $origem -Destination "$destino.anterior"
        Write-Warning "$destino ja existia e foi mantido; a versao antiga ficou em $destino.anterior"
    }
}

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
$conta = "NT SERVICE\$nome"
icacls $dataDir /inheritance:r /grant "*S-1-5-18:(OI)(CI)F" /grant "*S-1-5-32-544:(OI)(CI)F" | Out-Null

$servico = Get-Service -Name $nome -ErrorAction SilentlyContinue
if ($servico -and $servico.Status -ne "Stopped") {
    Stop-Service -Name $nome -Force -ErrorAction SilentlyContinue
    $servico.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
}

Invoke-Silencioso "schtasks /query /tn $nome"
if ($LASTEXITCODE -eq 0) {
    Write-Host "tarefa agendada '$nome' encontrada; o agente passa a rodar como servico"
    Invoke-Silencioso "schtasks /end /tn $nome"
    Invoke-Silencioso "schtasks /delete /f /tn $nome"
}
Stop-Process -Name $nome -Force -ErrorAction SilentlyContinue
if (Test-Path $cmdDest) {
    Remove-Item -Force -LiteralPath $cmdDest
}

Invoke-Silencioso "schtasks /query /tn $antigo"
$tarefaAntiga = ($LASTEXITCODE -eq 0)
if ($tarefaAntiga -or (Test-Path $installAntigo) -or (Test-Path $dataAntigo)) {
    Write-Host "instalacao antiga ($antigo) encontrada; migrando para $nome"
    Invoke-Silencioso "schtasks /end /tn $antigo"
    Invoke-Silencioso "schtasks /delete /f /tn $antigo"
    Stop-Process -Name $antigo -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1

    if (Test-Path $dataAntigo) {
        foreach ($arquivo in @("agent.env", "credential.json", "machine-id")) {
            $origem = Join-Path $dataAntigo $arquivo
            if (Test-Path $origem) {
                $destino = Join-Path $dataDir $arquivo
                $final = if (Test-Path $destino) { "$destino.anterior" } else { $destino }
                Move-SemSobrescrever $origem $destino
                if ($arquivo -eq "agent.env") {
                    (Get-Content -LiteralPath $final) -replace [regex]::Escape("\$antigo\"), "\$nome\" |
                        Set-Content -Encoding ascii -LiteralPath $final
                }
            }
        }
        $sobras = @(Get-ChildItem -Force -LiteralPath $dataAntigo)
        if ($sobras.Count -eq 0) {
            Remove-Item -Force -LiteralPath $dataAntigo
        } else {
            Write-Warning "$dataAntigo ainda tem $($sobras.Count) arquivo(s) nao reconhecido(s) e foi mantido; revise e apague"
        }
    }

    if (Test-Path $installAntigo) {
        Remove-Item -Recurse -Force -LiteralPath $installAntigo
    }
}

Invoke-Silencioso "icacls $dataDir\* /reset /T /C"

Copy-Item -Force $SourceExe $exeDest
Write-Host "binario instalado em $exeDest"

if (-not (Test-Path $envDest)) {
    @"
# Configuracao do agente de estacao do DockKeeper (Windows).
# Valores sem aspas e sem espacos.
# AGENT_ENROLL_TOKEN: convite de uso unico emitido pelo painel. Depois do primeiro
# boot a credencial fica em $dataDir\credential.json; apague o convite.

AGENT_SERVER_URL=https://painel.exemplo.com
AGENT_ENROLL_TOKEN=
AGENT_SITE=
AGENT_INTERVAL=5
#AGENT_HOSTNAME=
"@ | Set-Content -Encoding ascii $envDest
    Write-Host "config modelo criada em $envDest — preencha AGENT_ENROLL_TOKEN com o convite do painel antes de usar"
} else {
    Write-Host "config existente preservada: $envDest"
}

icacls $envDest /inheritance:r /grant "*S-1-5-18:F" /grant "*S-1-5-32-544:F" | Out-Null

if (-not (Get-Service -Name $nome -ErrorAction SilentlyContinue)) {
    New-Service -Name $nome -BinaryPathName "`"$exeDest`"" -DisplayName "DockKeeper - agente de estacao" `
        -Description "Envia as metricas desta estacao para o painel DockKeeper." -StartupType Automatic | Out-Null
    Write-Host "servico '$nome' criado"
}

& sc.exe config $nome obj= "$conta" password= "" | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Warning "nao foi possivel usar a conta virtual $conta; o servico segue como SYSTEM, com mais privilegio do que precisa"
} else {
    Write-Host "servico rodando como $conta"
    icacls $dataDir /grant "${conta}:(OI)(CI)M" | Out-Null
    icacls $envDest /grant "${conta}:R" | Out-Null
}

sc.exe failure $nome reset= 86400 actions= restart/30000/restart/30000/restart/60000 | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Warning "nao foi possivel configurar o reinicio em falha do servico" }
sc.exe failureflag $nome 1 | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Warning "nao foi possivel ligar o reinicio quando o servico para com erro" }

Start-Service -Name $nome
Start-Sleep -Seconds 2
Get-Service -Name $nome | Format-Table -AutoSize Name, Status, StartType
Write-Host "instalado como servico '$nome' (inicio automatico, reinicia em falha apos 30 s)"
Write-Host "verificar: Get-Service $nome"
