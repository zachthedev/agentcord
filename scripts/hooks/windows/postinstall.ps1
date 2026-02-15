# postinstall.ps1 — SessionStart hook.
# Outputs daemon status to stdout so Claude has context about plugin state.
# PowerShell port for Windows — invoked by Claude Code hook system.

if (-not $env:AGENTCORD_COMMON) { Write-Error 'AGENTCORD_COMMON not set — run via dispatch.ts'; exit 1 }
. $env:AGENTCORD_COMMON

# Drain stdin (Claude Code sends hook input on stdin)
[Console]::In.ReadToEnd() | Out-Null

# Check if daemon binary exists
if (-not (Test-Path $DaemonExe)) {
    Write-Output '[agentcord] Discord Rich Presence will activate on first tool use (daemon downloading)'
}
