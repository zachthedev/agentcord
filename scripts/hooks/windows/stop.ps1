# stop.ps1 — Stop hook.
# Marks presence as idle. Session cleanup is handled by sessionend.ps1.
# PowerShell port for Windows — invoked by Claude Code hook system.

if (-not $env:AGENTCORD_COMMON) { Write-Error 'AGENTCORD_COMMON not set — run via dispatch.ts'; exit 1 }
. $env:AGENTCORD_COMMON
Assert-JqInstalled
Read-HookInput

# ///////////////////////////////////////////////
# Mark Presence as Idle
# ///////////////////////////////////////////////

if (Test-Path $StatePath) {
    $currentSid = jq -r '.sessionId // empty' $StatePath
    if ($currentSid -eq $SessionId) {
        $now = [long][DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
        jq --argjson activity $now '.lastActivity=$activity|.stopped=true' $StatePath | Write-StateFile
    }
}
