/**
 * dispatch.ts — Cross-platform hook dispatcher.
 *
 * Usage:  bun dispatch.ts <hookname> <client>
 *
 * The client identifier (e.g. "claude-code") can be passed as the second
 * positional argument or via the AGENTCORD_CLIENT environment variable.
 * The positional argument is preferred for Windows compatibility — inline
 * env var syntax (VAR=val cmd) only works in Unix shells.
 *
 * Detects the current OS and delegates to the matching platform script:
 *   windows/<hookname>.ps1   (via pwsh)
 *   unix/<hookname>.sh       (via sh)
 *
 * Environment variables set for child hooks:
 *   AGENTCORD_LIB    — absolute path to lib/<platform> directory
 *   AGENTCORD_COMMON — full path to the shared common script
 *   AGENTCORD_CLIENT — client identifier (from argv[3] or env)
 *
 * Exit codes:
 *   0 — hook ran successfully (or no hook name provided)
 *   1 — AGENTCORD_CLIENT not set, or invalid hook name
 *
 * Contract:
 *   Hook failures are non-fatal — they are logged to stderr but never
 *   block the host tool.  The "postinstall" hook (SessionStart) uses
 *   inherited stdio; all other hooks redirect stdout/stderr to pipes
 *   so their output cannot leak into the host tool's JSON parser.
 */

import { execFileSync } from "child_process";
import { platform } from "os";
import { join } from "path";

const hook = process.argv[2];
if (!hook) {
  process.exit(0);
}
if (!/^[a-zA-Z0-9_-]+$/.test(hook)) {
  process.stderr.write(`invalid hook name: ${hook}\n`);
  process.exit(1);
}

const root = __dirname;
const client = process.argv[3] || process.env.AGENTCORD_CLIENT;
if (!client) {
  process.stderr.write("AGENTCORD_CLIENT not set — pass as 2nd arg or set env var\n");
  process.exit(1);
}

const isSessionStart = hook === "postinstall";
const stdio = isSessionStart ? "inherit" : (["pipe", "pipe", "inherit"] as const);

if (isSessionStart) {
  try {
    execFileSync("jq", ["--version"], { stdio: "pipe" });
  } catch {
    process.stderr.write("[agentcord] warning: jq not found — hooks may not work correctly\n");
  }
}

try {
  if (platform() === "win32") {
    const lib = join(root, "lib", "windows");
    execFileSync(
      "pwsh",
      ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", join(root, "windows", `${hook}.ps1`)],
      {
        stdio,
        env: {
          ...process.env,
          AGENTCORD_LIB: lib,
          AGENTCORD_COMMON: join(lib, "common.ps1"),
          AGENTCORD_CLIENT: client,
        },
      }
    );
  } else {
    const lib = join(root, "lib", "unix");
    execFileSync("sh", [join(root, "unix", `${hook}.sh`)], {
      stdio,
      env: {
        ...process.env,
        AGENTCORD_LIB: lib,
        AGENTCORD_COMMON: join(lib, "common.sh"),
        AGENTCORD_CLIENT: client,
      },
    });
  }
} catch (err: unknown) {
  const message = err instanceof Error ? err.message : String(err);
  process.stderr.write(`hook ${hook} failed: ${message}\n`);
  // Non-fatal — hook failures must not block the host tool
}
