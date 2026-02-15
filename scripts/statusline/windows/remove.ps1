# remove.ps1 — Restore original Claude Code statusline configuration.
# Reverses setup.ps1: restores the backed-up statusline or removes the key.

$ErrorActionPreference = 'Stop'

$DataDir = if ($env:AGENTCORD_DATA_DIR) { $env:AGENTCORD_DATA_DIR } else { Join-Path $HOME '.agentcord' }
$ClaudeConfig = Join-Path (Join-Path $HOME '.claude') 'settings.json'
$Backup = Join-Path $DataDir 'statusline-original.ps1'

if (-not (Test-Path $ClaudeConfig)) {
    Write-Host 'No Claude Code config found, nothing to restore.'
    exit 0
}

$cfg = Get-Content $ClaudeConfig -Raw | ConvertFrom-Json

if (Test-Path $Backup) {
    $cfg.statusline = $Backup
    Remove-Item $Backup -Force
    Write-Host 'Restored original statusline command.'
} else {
    $cfg.PSObject.Properties.Remove('statusline')
    Write-Host 'Removed Agentcord statusline configuration.'
}

$cfg | ConvertTo-Json -Depth 10 | Set-Content $ClaudeConfig

$StatuslineJson = Join-Path $DataDir 'statusline.json'
if (Test-Path $StatuslineJson) { Remove-Item $StatuslineJson -Force }

Write-Host 'Restart Claude Code for changes to take effect.'
