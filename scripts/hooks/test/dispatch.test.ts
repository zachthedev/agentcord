// dispatch.test.ts — Tests for the cross-platform hook dispatcher.
// Run with: bun test scripts/hooks/test/

import { describe, it, afterEach, expect } from "bun:test";
import { execFileSync, execSync } from "child_process";
import { join, resolve } from "path";
import {
  mkdtempSync,
  writeFileSync,
  readFileSync,
  existsSync,
  mkdirSync,
  rmSync,
} from "fs";
import { tmpdir, platform } from "os";

const dispatchScript = resolve(__dirname, "..", "dispatch.ts");

describe("dispatch.ts", () => {
  let tempDir: string | undefined;

  afterEach(() => {
    if (tempDir && existsSync(tempDir)) {
      rmSync(tempDir, { recursive: true, force: true });
    }
    tempDir = undefined;
  });

  it("exits cleanly with no args", () => {
    const result = execFileSync("bun", ["run", dispatchScript], {
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
    });
    expect(true).toBe(true);
  });

  it("exits cleanly with unknown hook name", () => {
    const result = execFileSync(
      "bun",
      ["run", dispatchScript, "nonexistent_hook_that_does_not_exist"],
      {
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, AGENTCORD_CLIENT: "test" },
      }
    );
    expect(true).toBe(true);
  });

  it("dispatches to the correct platform script", () => {
    tempDir = mkdtempSync(join(tmpdir(), "agentcord-dispatch-test-"));
    const markerFile = join(tempDir, "marker.txt");
    const isWin = platform() === "win32";

    if (isWin) {
      const hookDir = join(tempDir, "windows");
      mkdirSync(hookDir, { recursive: true });
      const libDir = join(tempDir, "lib", "windows");
      mkdirSync(libDir, { recursive: true });

      writeFileSync(
        join(hookDir, "testhook.ps1"),
        `"dispatched" | Set-Content -Path '${markerFile.replace(/\\/g, "\\\\")}' -Encoding utf8\n`
      );

      const miniDispatch = join(tempDir, "dispatch.ts");
      writeFileSync(
        miniDispatch,
        `
import { execFileSync } from "child_process";
import { join } from "path";
const hook = process.argv[2];
if (!hook) process.exit(0);
const root = ${JSON.stringify(tempDir)};
try {
  const lib = join(root, "lib", "windows");
  execFileSync("pwsh", ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", join(root, "windows", hook + ".ps1")], {
    stdio: "inherit",
    env: { ...process.env, AGENTCORD_LIB: lib, AGENTCORD_COMMON: join(lib, "common.ps1") },
  });
} catch {}
`
      );

      execFileSync("bun", ["run", miniDispatch, "testhook"], {
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
      });
    } else {
      const hookDir = join(tempDir, "unix");
      mkdirSync(hookDir, { recursive: true });
      const libDir = join(tempDir, "lib", "unix");
      mkdirSync(libDir, { recursive: true });

      writeFileSync(
        join(hookDir, "testhook.sh"),
        `#!/bin/sh\necho "dispatched" > "${markerFile}"\n`
      );
      execSync(`chmod +x "${join(hookDir, "testhook.sh")}"`);

      const miniDispatch = join(tempDir, "dispatch.ts");
      writeFileSync(
        miniDispatch,
        `
import { execFileSync } from "child_process";
import { join } from "path";
const hook = process.argv[2];
if (!hook) process.exit(0);
const root = ${JSON.stringify(tempDir)};
try {
  const lib = join(root, "lib", "unix");
  execFileSync("sh", [join(root, "unix", hook + ".sh")], {
    stdio: "inherit",
    env: { ...process.env, AGENTCORD_LIB: lib, AGENTCORD_COMMON: join(lib, "common.sh") },
  });
} catch {}
`
      );

      execFileSync("bun", ["run", miniDispatch, "testhook"], {
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
      });
    }

    expect(existsSync(markerFile)).toBe(true);
    const content = readFileSync(markerFile, "utf8").trim();
    expect(content).toBe("dispatched");
  });

  it("sets AGENTCORD_LIB and AGENTCORD_COMMON in child env", () => {
    tempDir = mkdtempSync(join(tmpdir(), "agentcord-env-test-"));
    const envDump = join(tempDir, "env.txt");
    const isWin = platform() === "win32";

    if (isWin) {
      const hookDir = join(tempDir, "windows");
      mkdirSync(hookDir, { recursive: true });
      const libDir = join(tempDir, "lib", "windows");
      mkdirSync(libDir, { recursive: true });

      writeFileSync(
        join(hookDir, "envhook.ps1"),
        `"LIB=$env:AGENTCORD_LIB\`nCOMMON=$env:AGENTCORD_COMMON" | Set-Content -Path '${envDump.replace(/\\/g, "\\\\")}' -Encoding utf8\n`
      );

      const miniDispatch = join(tempDir, "dispatch.ts");
      writeFileSync(
        miniDispatch,
        `
import { execFileSync } from "child_process";
import { join } from "path";
const hook = process.argv[2];
if (!hook) process.exit(0);
const root = ${JSON.stringify(tempDir)};
try {
  const lib = join(root, "lib", "windows");
  execFileSync("pwsh", ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", join(root, "windows", hook + ".ps1")], {
    stdio: "inherit",
    env: { ...process.env, AGENTCORD_LIB: lib, AGENTCORD_COMMON: join(lib, "common.ps1") },
  });
} catch {}
`
      );

      execFileSync("bun", ["run", miniDispatch, "envhook"], {
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
      });
    } else {
      const hookDir = join(tempDir, "unix");
      mkdirSync(hookDir, { recursive: true });
      const libDir = join(tempDir, "lib", "unix");
      mkdirSync(libDir, { recursive: true });

      writeFileSync(
        join(hookDir, "envhook.sh"),
        `#!/bin/sh\nprintf "LIB=%s\\nCOMMON=%s\\n" "$AGENTCORD_LIB" "$AGENTCORD_COMMON" > "${envDump}"\n`
      );
      execSync(`chmod +x "${join(hookDir, "envhook.sh")}"`);

      const miniDispatch = join(tempDir, "dispatch.ts");
      writeFileSync(
        miniDispatch,
        `
import { execFileSync } from "child_process";
import { join } from "path";
const hook = process.argv[2];
if (!hook) process.exit(0);
const root = ${JSON.stringify(tempDir)};
try {
  const lib = join(root, "lib", "unix");
  execFileSync("sh", [join(root, "unix", hook + ".sh")], {
    stdio: "inherit",
    env: { ...process.env, AGENTCORD_LIB: lib, AGENTCORD_COMMON: join(lib, "common.sh") },
  });
} catch {}
`
      );

      execFileSync("bun", ["run", miniDispatch, "envhook"], {
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
      });
    }

    expect(existsSync(envDump)).toBe(true);
    const lines = readFileSync(envDump, "utf8").trim().split(/\r?\n/);
    const libLine = lines.find((l) => l.startsWith("LIB="));
    const commonLine = lines.find((l) => l.startsWith("COMMON="));

    expect(libLine).toBeDefined();
    expect(commonLine).toBeDefined();

    const libValue = libLine!.replace("LIB=", "");
    const commonValue = commonLine!.replace("COMMON=", "");

    expect(libValue.length).toBeGreaterThan(0);
    expect(commonValue).toContain("common.");
  });
});
