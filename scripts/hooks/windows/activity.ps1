# activity.ps1 — PreToolUse / PostToolUse hook.
# Updates session state and ensures the daemon is running.
# PowerShell port for Windows — invoked by Claude Code hook system.

if (-not $env:AGENTCORD_COMMON) { Write-Error 'AGENTCORD_COMMON not set — run via dispatch.ts'; exit 1 }
. $env:AGENTCORD_COMMON
Assert-JqInstalled
Read-HookInput

# Debounce: skip if state was written less than 5 seconds ago
if (Test-Path $StatePath) {
    $lastWrite = (Get-Item $StatePath).LastWriteTime
    if (([DateTime]::Now - $lastWrite).TotalSeconds -lt 5) { exit 0 }
}

if (-not (Test-Path $SessionsPath)) {
    New-Item -ItemType Directory -Path $SessionsPath -Force | Out-Null
}

# ///////////////////////////////////////////////
# Session State
# ///////////////////////////////////////////////

# Clean up orphaned session markers older than 24 hours
Get-ChildItem -Path $SessionsPath -Filter "*$SessionExt" -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt (Get-Date).AddHours(-24) } | Remove-Item -Force -ErrorAction SilentlyContinue

# Write session marker file
'' | Set-Content (Join-Path $SessionsPath "$SessionId$SessionExt")

# Read existing state to preserve sessionStart
$SessionStart = $null
if (Test-Path $StatePath) {
    $oldSid = jq -r '.sessionId // empty' $StatePath
    if ($oldSid -eq $SessionId) {
        $SessionStart = jq -r '.sessionStart // empty' $StatePath
    }
}

# New session or no prior state — set fresh start time
if (-not $SessionStart) {
    $SessionStart = [long][DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
}

$LastActivity = [long][DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$Project = Split-Path $PWD -Leaf
$Cwd = $PWD.Path

# ///// Git Info /////

$Branch = ''
$GitRemote = ''
try {
    $b = git rev-parse --abbrev-ref HEAD 2>$null
    if ($b) { $Branch = $b.Trim() }
} catch {}
try {
    $r = git remote get-url origin 2>$null
    if ($r) { $GitRemote = $r.Trim() }
} catch {}

# ///// Tool Context /////

$ToolName = $HookInput | jq -r '.tool_name // empty'
$HookEvent = $HookInput | jq -r '.hook_event_name // empty'
$PermissionMode = $HookInput | jq -r '.permission_mode // empty'

$ToolTarget = ''
$ActiveFile = ''
switch ($ToolName) {
    { $_ -in 'Edit', 'Write', 'Read' } {
        $ToolTarget = $HookInput | jq -r '.tool_input.file_path // empty'
        $ActiveFile = $ToolTarget
    }
    'Bash' {
        $raw = $HookInput | jq -r '.tool_input.command // empty'
        $ToolTarget = if ($raw.Length -gt 80) { $raw.Substring(0, 80) } else { $raw }
    }
    { $_ -in 'Grep', 'Glob' } {
        $ToolTarget = $HookInput | jq -r '.tool_input.pattern // empty'
    }
}

$AgentState = switch ($HookEvent) {
    'PreToolUse'       { 'tool' }
    'PostToolUse'      { 'thinking' }
    'UserPromptSubmit' { 'thinking' }
    'Notification'     { 'waiting' }
    default            { '' }
}

if ($HookEvent -in 'PostToolUse', 'PostToolUseFailure') {
    $ToolName = ''
    $ToolTarget = ''
}

# ///// Write State /////

jq -n `
    --argjson version $StateVersion `
    --arg client $Client `
    --arg sid $SessionId `
    --argjson start $SessionStart `
    --argjson activity $LastActivity `
    --arg project $Project `
    --arg branch $Branch `
    --arg cwd $Cwd `
    --arg remote $GitRemote `
    --arg toolName $ToolName `
    --arg toolTarget $ToolTarget `
    --arg activeFile $ActiveFile `
    --arg agentState $AgentState `
    --arg permissionMode $PermissionMode `
    --arg hookEvent $HookEvent `
    '{\"$version\": $version, \"sessionId\": $sid, \"sessionStart\": $start, \"lastActivity\": $activity, \"project\": $project, \"branch\": $branch, \"cwd\": $cwd, \"gitRemoteUrl\": $remote, \"client\": $client, \"stopped\": false, \"toolName\": $toolName, \"toolTarget\": $toolTarget, \"activeFile\": $activeFile, \"agentState\": $agentState, \"permissionMode\": $permissionMode, \"hookEvent\": $hookEvent}' | Write-StateFile

# ///////////////////////////////////////////////
# Daemon Health Check
# ///////////////////////////////////////////////

$DaemonAlive = $false
if (Test-Path $PidPath) {
    $pidContent = (Get-Content $PidPath -Raw).Trim()
    $DaemonPid = ($pidContent -split ':')[0]
    try {
        Get-Process -Id $DaemonPid -ErrorAction Stop | Out-Null
        $DaemonAlive = $true
    } catch {
        $DaemonAlive = $false
    }
}

if (-not $DaemonAlive) {
    # Download binary if missing
    if (-not (Test-Path $DaemonExe)) {
        $pluginRoot = if ($env:CLAUDE_PLUGIN_ROOT) { $env:CLAUDE_PLUGIN_ROOT } else { Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)) }
        $installScript = Join-Path (Join-Path $pluginRoot 'scripts') 'install.ps1'
        if (Test-Path $installScript) {
            & pwsh -NoProfile -ExecutionPolicy Bypass -File $installScript
        }
    }

    # Launch daemon in background
    if (Test-Path $DaemonExe) {
        Start-Process -FilePath $DaemonExe -ArgumentList '--data-dir', $DataDir -WindowStyle Hidden
    }
}
