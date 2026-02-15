# setup.ps1 — Configure Claude Code to pipe statusline data to Agentcord.
# Backs up any existing statusline command, then sets the wrapper as the handler.
# Requires jq to be installed. Idempotent — safe to run multiple times.

$ErrorActionPreference = 'Stop'

# ///////////////////////////////////////////////
# Prerequisites
# ///////////////////////////////////////////////

if (-not (Get-Command jq -ErrorAction SilentlyContinue)) {
    Write-Error 'jq is required but not installed. Install via: winget install jqlang.jq'
    exit 1
}

# ///////////////////////////////////////////////
# Paths
# ///////////////////////////////////////////////

$DataDir = if ($env:AGENTCORD_DATA_DIR) { $env:AGENTCORD_DATA_DIR } else { Join-Path $HOME '.agentcord' }
$ClaudeConfig = Join-Path (Join-Path $HOME '.claude') 'settings.json'
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Wrapper = Join-Path $ScriptDir 'wrapper.ps1'

if (-not (Test-Path $DataDir)) { New-Item -ItemType Directory -Path $DataDir -Force | Out-Null }

# ///////////////////////////////////////////////
# Backup existing statusline
# ///////////////////////////////////////////////

if (Test-Path $ClaudeConfig) {
    $existing = & jq -r '.statusline // empty' $ClaudeConfig 2>$null
    if ($existing -and $existing -ne $Wrapper) {
        Copy-Item $existing (Join-Path $DataDir 'statusline-original.ps1') -Force -ErrorAction SilentlyContinue
        Write-Host "Backed up existing statusline to $(Join-Path $DataDir 'statusline-original.ps1')"
    }
}

# ///////////////////////////////////////////////
# Configure Claude Code
# ///////////////////////////////////////////////

if (-not (Test-Path $ClaudeConfig)) {
    '{}' | Set-Content $ClaudeConfig -Encoding utf8
}

$tmp = "$ClaudeConfig.tmp.$PID"
& jq --arg wrapper $Wrapper '.statusline = $wrapper' $ClaudeConfig | Set-Content $tmp -Encoding utf8
Move-Item -Path $tmp -Destination $ClaudeConfig -Force

Write-Host 'Claude Code statusline configured to use Agentcord wrapper.'
Write-Host 'Restart Claude Code for changes to take effect.'
