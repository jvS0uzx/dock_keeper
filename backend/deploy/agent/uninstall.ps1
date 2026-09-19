$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "rode este script num PowerShell como Administrador"
    exit 1
}

$nome   = "dockkeeper-agent"
$antigo = "vd-agent"

function Invoke-Silencioso([string]$linha) {
    cmd.exe /c "$linha >nul 2>&1" | Out-Null
}

if (Get-Service -Name $nome -ErrorAction SilentlyContinue) {
    Stop-Service -Name $nome -Force -ErrorAction SilentlyContinue
    sc.exe delete $nome | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Warning "nao foi possivel remover o servico $nome" }
}

foreach ($tarefa in @($nome, $antigo)) {
    Invoke-Silencioso "schtasks /end /tn $tarefa"
    Invoke-Silencioso "schtasks /delete /f /tn $tarefa"
    Stop-Process -Name $tarefa -Force -ErrorAction SilentlyContinue
}
Start-Sleep -Seconds 1

foreach ($dir in @("C:\Program Files\$nome", "C:\Program Files\$antigo")) {
    if (Test-Path $dir) {
        Remove-Item -Recurse -Force -LiteralPath $dir
    }
}
Write-Host "servico e binario removidos"

$dadosNovos   = "C:\ProgramData\$nome"
$dadosAntigos = "C:\ProgramData\$antigo"
$dados = @(@($dadosNovos, $dadosAntigos) | Where-Object { Test-Path $_ })

if ($dados.Count -gt 0) {
    $resposta = Read-Host "apagar tambem config e credencial em $($dados -join ', ')? [s/N]"
    if ($resposta -eq "s" -or $resposta -eq "S") {
        foreach ($dir in $dados) {
            Remove-Item -Recurse -Force -LiteralPath $dir
        }
        Write-Host "config removida"
    } else {
        Write-Host "config preservada em $($dados -join ', ')"
    }
}
