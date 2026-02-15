# Adding a New Tool Integration

This guide walks you through adding Agentcord support for a new agentic coding tool.

## Step 1: Choose a client ID

Client IDs must match: `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` (max 48 chars).

Examples: `cursor`, `windsurf`, `claude-code`, `roo-code`, `continue-jb`

IDE variant suffixes: `-jb` (JetBrains), `-vim` (Neovim), `-web` (browser), `-vsc` (VS Code, only when disambiguation needed).

## Step 2: Create your client directory

```
clients/
  your-tool/
    README.md           # Document your tool's data sources and setup
    hooks.json          # If your tool supports hooks (optional)
```

## Step 3: Write state to the daemon

Your integration needs to write a JSON state file to `~/.agentcord/state.{client-id}.json`.

### State file schema

```json
{
  "$version": 1,
  "sessionId": "unique-session-id",
  "sessionStart": 1700000000,
  "lastActivity": 1700000060,
  "project": "my-project",
  "branch": "main",
  "cwd": "/path/to/project",
  "gitRemoteUrl": "https://github.com/user/repo",
  "client": "your-tool",
  "stopped": false,
  "toolName": "Edit",
  "toolTarget": "/path/to/file.ts",
  "activeFile": "/path/to/file.ts",
  "agentState": "tool",
  "permissionMode": "",
  "hookEvent": "PreToolUse"
}
```

Required fields: `$version`, `sessionId`, `sessionStart`, `lastActivity`, `project`, `client`, `stopped`.

Optional fields: `branch`, `cwd`, `gitRemoteUrl`, `toolName`, `toolTarget`, `activeFile`, `agentState`, `permissionMode`, `hookEvent`.

### Integration methods

1. **Hook scripts** (preferred): If your tool has a hook system, use `dispatch.ts` to delegate to platform scripts. Copy `hooks.json.example` and adapt.
2. **VS Code extension**: If your tool is a VS Code fork/extension, write a small extension that writes state on editor events.
3. **Direct write**: Any process that writes valid state JSON works.

## Step 4: Register your client

Add your tool to the `knownClients` map in `internal/config/config.go`:

```go
"your-tool": {DisplayName: "Your Tool", Icon: "your_tool"},
```

The icon name uses underscores (Discord asset key convention). Upload a matching icon to the Discord app's Rich Presence assets.

## Step 5: Add session ID field mapping

If your tool uses a different field name for session IDs, add a case to:
- `scripts/hooks/lib/unix/common.sh`
- `scripts/hooks/lib/windows/common.ps1`

## Step 6: Test

```bash
# Build and run
make build
./bin/agentcord --data-dir ~/.agentcord

# Write a test state file
echo '{"$version":1,"sessionId":"test","sessionStart":1700000000,"lastActivity":'$(date +%s)',"project":"test","client":"your-tool","stopped":false}' > ~/.agentcord/state.your-tool.json

# Check daemon logs
tail -f ~/.agentcord/agentcord.log
```

## Step 7: Submit a PR

Include:
- Your `clients/your-tool/` directory with README
- Updated `knownClients` map in `internal/config/config.go`
- Session ID field mapping (if needed)
- Any hook configs or extension code
