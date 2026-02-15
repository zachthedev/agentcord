# sessionend.ps1 — SessionEnd hook.
# Cleans up session marker and kills daemon when no sessions remain.
# PowerShell port for Windows — invoked by Claude Code hook system.

if (-not $env:AGENTCORD_COMMON) { Write-Error 'AGENTCORD_COMMON not set — run via dispatch.ts'; exit 1 }
. $env:AGENTCORD_COMMON
Assert-JqInstalled
Read-HookInput

# ///////////////////////////////////////////////
# Session Cleanup
# ///////////////////////////////////////////////

# Remove this session's marker file
$marker = Join-Path $SessionsPath "$SessionId$SessionExt"
if (Test-Path $marker) { Remove-Item $marker -Force }

# Count remaining sessions
$remaining = @(Get-ChildItem -Path $SessionsPath -Filter "*$SessionExt" -File -ErrorAction SilentlyContinue).Count

# ///////////////////////////////////////////////
# Last Session — Stop Daemon
# ///////////////////////////////////////////////

if ($remaining -eq 0) {
    $now = [long][DateTimeOffset]::UtcNow.ToUnixTimeSeconds()

    if (Test-Path $StatePath) {
        jq --argjson activity $now '.lastActivity=$activity|.stopped=true' $StatePath | Write-StateFile
    } else {
        jq -n `
            --argjson version $StateVersion `
            --arg client $Client `
            --arg sid $SessionId `
            --argjson activity $now `
            '{\"$version\":$version,\"sessionId\":$sid,\"sessionStart\":$activity,\"lastActivity\":$activity,\"project\":\"\",\"branch\":\"\",\"cwd\":\"\",\"gitRemoteUrl\":\"\",\"client\":$client,\"stopped\":true}' | Write-StateFile
    }

    Stop-AgentcordDaemon
}
