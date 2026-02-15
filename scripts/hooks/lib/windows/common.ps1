# common.ps1 — Shared config and functions for agentcord hook scripts.
# Sourced via: . "$env:AGENTCORD_LIB\common.ps1"

$ErrorActionPreference = 'Continue'

# Load generated constants (source of truth: internal/paths/paths.go)
. (Join-Path $PSScriptRoot 'constants.ps1')

# Construct full paths from constants
$DataDir      = $(if ($env:AGENTCORD_DATA_DIR) { $env:AGENTCORD_DATA_DIR } else { Join-Path $HOME $DataDirRel })
$PidPath      = Join-Path $DataDir $PidFile
$SessionsPath = Join-Path $DataDir $SessionsDir
$DaemonExe    = Join-Path $DataDir "$BinaryName.exe"

# Client identity — set by dispatch.ts, required
if (-not $env:AGENTCORD_CLIENT) { Write-Error 'AGENTCORD_CLIENT not set — run via dispatch.ts'; exit 1 }
$Client = $env:AGENTCORD_CLIENT

# Session ID field — varies by client tool
$SessionIdField = switch ($Client) {
    'claude-code' { '.session_id' }
    default       { if ($env:AGENTCORD_SESSION_ID_FIELD) { $env:AGENTCORD_SESSION_ID_FIELD } else { '.session_id' } }
}

# Per-client state file path
$StatePath    = Join-Path $DataDir "state.$Client.json"

function Assert-JqInstalled {
    if (-not (Get-Command jq -ErrorAction SilentlyContinue)) {
        Write-Error 'jq is required for agentcord hooks'
        exit 1
    }
}

function Read-HookInput {
    $script:HookInput = [Console]::In.ReadToEnd()
    $script:SessionId = $HookInput | jq -r "$SessionIdField // empty"
    if (-not $SessionId -or $SessionId -eq 'null') { exit 0 }
}

function Write-StateFile {
    param([Parameter(ValueFromPipeline)]$Content)
    $tmp = "$StatePath.tmp.$PID"
    $Content | Set-Content $tmp -Encoding utf8
    Move-Item -Path $tmp -Destination $StatePath -Force
}

function Stop-AgentcordDaemon {
    if (Test-Path $PidPath) {
        $pidContent = (Get-Content $PidPath -Raw).Trim()
        $DaemonPid = ($pidContent -split ':')[0]
        try { Stop-Process -Id $DaemonPid -Force -ErrorAction Stop } catch {}
        Remove-Item $PidPath -Force
    }
}
