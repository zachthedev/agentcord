param(
    [string]$Client = 'code'
)

# install.ps1 — Auto-download agentcord binary from GitHub Releases.
# PowerShell port for Windows. Called by activity.ps1 when the daemon binary is missing,
# or manually for setup. Must NOT use $ErrorActionPreference = 'Stop' in hook context,
# but standalone install can use it.

$ErrorActionPreference = 'Stop'

# ///////////////////////////////////////////////
# Configuration
# ///////////////////////////////////////////////

$DataDir = if ($env:AGENTCORD_DATA_DIR) { $env:AGENTCORD_DATA_DIR } else { Join-Path $HOME '.agentcord' }
$DryRun = $env:AGENTCORD_DOWNLOAD_DRY_RUN -eq 'true'

# ///////////////////////////////////////////////
# Platform Detection
# ///////////////////////////////////////////////

$rawArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
switch ($rawArch) {
    'X64'   { $goArch = 'amd64' }
    'Arm64' { $goArch = 'arm64' }
    default {
        Write-Error "agentcord: unsupported architecture: $rawArch"
        exit 1
    }
}

# ///////////////////////////////////////////////
# Download
# ///////////////////////////////////////////////

$binaryName = "agentcord-windows-$goArch.exe"
$url = "https://github.com/zachthedev/agentcord/releases/latest/download/$binaryName"

if ($DryRun) {
    Write-Output $url
    exit 0
}

if (-not (Test-Path $DataDir)) {
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
}

$outPath = Join-Path $DataDir 'agentcord.exe'

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $url -OutFile $outPath -UseBasicParsing
} catch {
    Write-Error "agentcord: failed to download from $url"
    Remove-Item $outPath -Force -ErrorAction SilentlyContinue
    exit 1
}

# ///////////////////////////////////////////////
# Checksum Verification
# ///////////////////////////////////////////////

$checksumUrl = "https://github.com/zachthedev/agentcord/releases/latest/download/checksums.txt"
$checksumFile = Join-Path $DataDir 'checksums.txt'

try {
    Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumFile -UseBasicParsing
} catch {
    Write-Error "agentcord: failed to download checksums"
    Remove-Item $outPath -Force -ErrorAction SilentlyContinue
    Remove-Item $checksumFile -Force -ErrorAction SilentlyContinue
    exit 1
}

$checksumLines = Get-Content $checksumFile
$matchLine = $checksumLines | Where-Object { $_ -match [regex]::Escape($binaryName) } | Select-Object -First 1

if (-not $matchLine) {
    Write-Error "agentcord: no checksum found for $binaryName"
    Remove-Item $checksumFile -Force -ErrorAction SilentlyContinue
    exit 1
}

$expected = ($matchLine -split '\s+')[0].ToLower()
$actual = (Get-FileHash -Path $outPath -Algorithm SHA256).Hash.ToLower()

if ($expected -ne $actual) {
    Write-Error "agentcord: checksum mismatch! Expected: $expected, Got: $actual"
    Remove-Item $outPath -Force -ErrorAction SilentlyContinue
    Remove-Item $checksumFile -Force -ErrorAction SilentlyContinue
    exit 1
}

Remove-Item $checksumFile -Force -ErrorAction SilentlyContinue

Write-Host "agentcord: downloaded daemon to $outPath" -ForegroundColor Green

# ///////////////////////////////////////////////
# Client Hooks Directory
# ///////////////////////////////////////////////

$HooksDir = switch ($Client) {
    'code'     { Join-Path $HOME '.claude' }
    'cursor'   { Join-Path $HOME '.cursor' }
    'windsurf' { Join-Path $HOME '.windsurf' }
    default    { Write-Error "Unknown client: $Client"; exit 1 }
}

# ///////////////////////////////////////////////
# Statusline Auto-Setup
# ///////////////////////////////////////////////

$pluginRoot = if ($env:CLAUDE_PLUGIN_ROOT) { $env:CLAUDE_PLUGIN_ROOT } else { Split-Path -Parent $PSScriptRoot }
$setupScript = Join-Path (Join-Path (Join-Path $pluginRoot 'scripts') 'statusline') (Join-Path 'windows' 'setup.ps1')

if (Test-Path $setupScript) {
    try {
        & pwsh -NoProfile -ExecutionPolicy Bypass -File $setupScript
    } catch {
        Write-Host "agentcord: statusline setup failed (non-fatal): $_" -ForegroundColor Yellow
    }
}
