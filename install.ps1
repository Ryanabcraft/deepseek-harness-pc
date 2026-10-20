#Requires -Version 5.1
<#
    DeepSeek Harness PC — installer
    Usage:  irm https://raw.githubusercontent.com/Ryanabcraft/deepseek-harness-pc/main/install.ps1 | iex
#>
[CmdletBinding()]
param(
    [switch]$Yes,
    [switch]$NoShortcut
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo       = 'Ryanabcraft/deepseek-harness-pc'
$AppName    = 'DeepSeek Harness'
$ExeName    = 'DeepSeek Harness.exe'
$AssetName  = 'DeepSeek-Harness.exe'
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\DeepSeekHarness'

function Write-Step($m) { Write-Host "`n==> $m" -ForegroundColor Cyan }
function Write-Ok($m)   { Write-Host "    ok   $m" -ForegroundColor Green }
function Write-Skip($m) { Write-Host "    skip $m" -ForegroundColor DarkGray }
function Write-Note($m) { Write-Host "    !    $m" -ForegroundColor Yellow }

function Update-SessionPath {
    $machine = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $user    = [Environment]::GetEnvironmentVariable('Path', 'User')
    $env:Path = "$machine;$user"
}

function Test-Cmd($name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

Write-Host @"

   ██████╗ ██████╗ ███████╗██╗  ██╗
   ██╔══██╗██╔══██╗██╔════╝██║ ██╔╝
   ██║  ██║██████╔╝███████╗█████╔╝
   ██║  ██║██╔═══╝ ╚════██║██╔═██╗
   ██████╔╝██║     ███████║██║  ██╗
   ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═╝

   D E E P S E E K   H A R N E S S   ·   installer
"@ -ForegroundColor Cyan

# ---------------------------------------------------------------- Node.js
Write-Step 'Node.js'
if (Test-Cmd node) {
    Write-Skip "já instalado ($(node --version))"
}
else {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw 'Node.js não encontrado e winget indisponível. Instale Node.js LTS manualmente: https://nodejs.org'
    }
    Write-Host '    instalando Node.js LTS via winget...'
    winget install --id OpenJS.NodeJS.LTS -e --silent --accept-package-agreements --accept-source-agreements | Out-Null
    Update-SessionPath
    if (-not (Test-Cmd node) -and (Test-Path "$env:ProgramFiles\nodejs\node.exe")) {
        $env:Path = "$env:ProgramFiles\nodejs;$env:Path"
    }
    if (-not (Test-Cmd node)) { throw 'Node.js foi instalado mas não está no PATH desta sessão. Feche e abra o terminal e rode de novo.' }
    Write-Ok "Node.js $(node --version)"
}

# ---------------------------------------------------------------- DSH
Write-Step 'DeepSeek Harness (dsh)'
if (Test-Cmd dsh) {
    Write-Skip 'dsh já instalado'
}
else {
    Write-Host '    instalando @deepseek-ai/dsh...'
    & npm.cmd install -g @deepseek-ai/dsh
    Update-SessionPath
    if (-not (Test-Cmd dsh)) { throw 'dsh foi instalado mas não está no PATH desta sessão. Feche e abra o terminal e rode de novo.' }
    Write-Ok 'dsh instalado'
}

# ---------------------------------------------------------------- OpenCode
Write-Step 'OpenCode CLI'
if (Test-Cmd opencode) {
    Write-Skip 'opencode já instalado'
}
else {
    Write-Note 'Opcional: o bridge de modelos free fala direto com a API pública do OpenCode, sem CLI local.'
    $answer = 'n'
    if ($Yes) { $answer = 's' }
    elseif ([Environment]::UserInteractive) {
        $answer = Read-Host '    Instalar OpenCode CLI agora? (s/N)'
    }
    if ($answer -match '^(s|sim|y|yes)$') {
        Write-Host '    instalando @opencode/cli...'
        & npm.cmd install -g @opencode/cli
        Write-Ok 'opencode instalado'
    }
    else {
        Write-Skip 'seguindo sem OpenCode (não é obrigatório)'
    }
}

# ---------------------------------------------------------------- App
Write-Step "$AppName app"
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
$target = Join-Path $InstallDir $ExeName
$url    = "https://github.com/$Repo/releases/latest/download/$AssetName"

Write-Host "    baixando $url"
try {
    Invoke-WebRequest -Uri $url -OutFile $target -UseBasicParsing
}
catch {
    throw "Falha ao baixar o app ($url). Verifique se existe um release publicado. Detalhe: $($_.Exception.Message)"
}
Write-Ok "instalado em $target"

if (-not $NoShortcut) {
    $desktop = [Environment]::GetFolderPath('Desktop')
    $lnkPath = Join-Path $desktop "$AppName.lnk"
    $shell   = New-Object -ComObject WScript.Shell
    $lnk     = $shell.CreateShortcut($lnkPath)
    $lnk.TargetPath       = $target
    $lnk.WorkingDirectory = $InstallDir
    $lnk.IconLocation     = "$target,0"
    $lnk.Description      = 'DeepSeek Harness web console'
    $lnk.Save()
    Write-Ok "atalho criado: $lnkPath"
}

Write-Host "`nPronto. Abra '$AppName' no Desktop." -ForegroundColor Green
Write-Host 'Na primeira execução o app sobe o dsh web e abre a janela nativa.' -ForegroundColor DarkGray
