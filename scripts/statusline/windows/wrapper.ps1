# wrapper.ps1 — Pipe Claude Code statusline JSON to a file for Agentcord.
# Reads JSON from stdin, writes atomically to {dataDir}/statusline.json,
# then chains to the original statusline command if one was backed up.

$DataDir = if ($env:AGENTCORD_DATA_DIR) { $env:AGENTCORD_DATA_DIR } else { Join-Path $HOME '.agentcord' }
$Input = $input | Out-String

# Atomic write: temp + Move-Item
$TmpPath = Join-Path $DataDir "statusline.json.tmp.$PID"
$TargetPath = Join-Path $DataDir 'statusline.json'
[System.IO.File]::WriteAllText($TmpPath, $Input.Trim())
Move-Item -Path $TmpPath -Destination $TargetPath -Force

# Chain to original statusline if backed up
$Backup = Join-Path $DataDir 'statusline-original.ps1'
if (Test-Path $Backup) {
    $Input | & $Backup
}
