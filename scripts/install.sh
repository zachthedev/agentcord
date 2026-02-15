#!/usr/bin/env bash
# install.sh — Auto-download agentcord binary from GitHub Releases.
# Called by hook-activity.sh when the daemon binary is missing,
# or by start.sh for manual daemon setup.
# Must NOT use set -e (called from hook context).

# ///////////////////////////////////////////////
# Arguments
# ///////////////////////////////////////////////

INSTALL_CLIENT="code"
while [ $# -gt 0 ]; do
    case "$1" in
        --client) INSTALL_CLIENT="$2"; shift 2 ;;
        *) shift ;;
    esac
done

# ///////////////////////////////////////////////
# Configuration
# ///////////////////////////////////////////////

BASE_NAME="agentcord"
DATA_DIR_REL=".agentcord"
DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/$DATA_DIR_REL}"

# Dry-run mode for testing: print URL and exit
DRY_RUN="${AGENTCORD_DOWNLOAD_DRY_RUN:-false}"

# ///////////////////////////////////////////////
# Cleanup Trap
# ///////////////////////////////////////////////

TMP_FILE=""
cleanup() { rm -f "$TMP_FILE" 2>/dev/null; }
trap cleanup EXIT

# ///////////////////////////////////////////////
# Platform Detection
# ///////////////////////////////////////////////

# ///// OS /////

# Check for WSL (reports as Linux but runs on Windows)
if [ -f /proc/version ] && grep -qi microsoft /proc/version; then
    GOOS="linux"  # WSL should use Linux binary
else
    RAW_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$RAW_OS" in
        linux)   GOOS="linux" ;;
        darwin)  GOOS="darwin" ;;
        *_nt*|*mingw*|*msys*|*cygwin*)
                 GOOS="windows" ;;
        *)
            echo "agentcord: unsupported OS: $RAW_OS" >&2
            exit 1
            ;;
    esac
fi

# ///// Architecture /////

RAW_ARCH=$(uname -m)
case "$RAW_ARCH" in
    x86_64|amd64)   GOARCH="amd64" ;;
    aarch64|arm64)   GOARCH="arm64" ;;
    *)
        echo "agentcord: unsupported architecture: $RAW_ARCH" >&2
        exit 1
        ;;
esac

# ///////////////////////////////////////////////
# Download
# ///////////////////////////////////////////////

BINARY_NAME="${BASE_NAME}-${GOOS}-${GOARCH}"
if [ "$GOOS" = "windows" ]; then
    BINARY_NAME="${BINARY_NAME}.exe"
fi
URL="https://github.com/zachthedev/agentcord/releases/latest/download/${BINARY_NAME}"

if [ "$DRY_RUN" = "true" ]; then
    echo "$URL"
    exit 0
fi

mkdir -p "$DATA_DIR"

OUT_NAME="$BASE_NAME"
if [ "$GOOS" = "windows" ]; then
    OUT_NAME="${BASE_NAME}.exe"
fi
OUT_PATH="$DATA_DIR/$OUT_NAME"

if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$OUT_PATH" "$URL" || { echo "agentcord: failed to download from $URL" >&2; rm -f "$OUT_PATH"; exit 1; }
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$OUT_PATH" "$URL" || { echo "agentcord: failed to download from $URL" >&2; rm -f "$OUT_PATH"; exit 1; }
else
    echo "agentcord: neither curl nor wget available" >&2
    exit 1
fi

if [ ! -s "$OUT_PATH" ]; then
    echo "agentcord: downloaded binary is empty" >&2
    exit 1
fi

# ///////////////////////////////////////////////
# Checksum Verification
# ///////////////////////////////////////////////

CHECKSUM_URL="https://github.com/zachthedev/agentcord/releases/latest/download/checksums.txt"
CHECKSUM_FILE="$DATA_DIR/checksums.txt"
TMP_FILE="$CHECKSUM_FILE"

if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$CHECKSUM_FILE" "$CHECKSUM_URL" || { echo "agentcord: failed to download checksums" >&2; rm -f "$OUT_PATH" "$CHECKSUM_FILE"; exit 1; }
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$CHECKSUM_FILE" "$CHECKSUM_URL" || { echo "agentcord: failed to download checksums" >&2; rm -f "$OUT_PATH" "$CHECKSUM_FILE"; exit 1; }
fi

EXPECTED=$(grep "${BINARY_NAME}" "$CHECKSUM_FILE" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    echo "agentcord: no checksum found for ${BINARY_NAME}" >&2
    rm -f "$CHECKSUM_FILE"
    exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$OUT_PATH" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "$OUT_PATH" | awk '{print $1}')
else
    echo "agentcord: no sha256 tool found, cannot verify checksum" >&2
    rm -f "$OUT_PATH" "$CHECKSUM_FILE"
    exit 1
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "agentcord: checksum mismatch! Expected: $EXPECTED, Got: $ACTUAL" >&2
    rm -f "$OUT_PATH" "$CHECKSUM_FILE"
    exit 1
fi

rm -f "$CHECKSUM_FILE"

# Make executable (skip on Windows — not needed and may fail)
if [ "$GOOS" != "windows" ]; then
    chmod +x "$OUT_PATH"
fi

echo "agentcord: downloaded daemon to $OUT_PATH" >&2

# ///////////////////////////////////////////////
# Client Hooks Directory
# ///////////////////////////////////////////////

case "$INSTALL_CLIENT" in
    code)     HOOKS_DIR="$HOME/.claude" ;;
    cursor)   HOOKS_DIR="$HOME/.cursor" ;;
    windsurf) HOOKS_DIR="$HOME/.windsurf" ;;
    *)        echo "Unknown client: $INSTALL_CLIENT" >&2; exit 1 ;;
esac

# ///////////////////////////////////////////////
# Statusline Auto-Setup
# ///////////////////////////////////////////////

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/..}"

if [ "$GOOS" = "windows" ]; then
    # Windows: use PowerShell setup
    SETUP_SCRIPT="$PLUGIN_ROOT/scripts/statusline/windows/setup.ps1"
    if [ -f "$SETUP_SCRIPT" ] && command -v pwsh >/dev/null 2>&1; then
        pwsh -NoProfile -ExecutionPolicy Bypass -File "$SETUP_SCRIPT" 2>/dev/null || true
    fi
else
    # Unix/macOS: use bash setup
    SETUP_SCRIPT="$PLUGIN_ROOT/scripts/statusline/unix/setup.sh"
    if [ -f "$SETUP_SCRIPT" ]; then
        sh "$SETUP_SCRIPT" 2>/dev/null || true
    fi
fi
