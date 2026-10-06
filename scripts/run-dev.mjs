#!/usr/bin/env node
// `node scripts/run-dev.mjs <args>`: runs the CLI from this checkout
// with `go run ./cmd/wspace`, isolated from the configuration of an
// installed wspace. Unless WSPACE_CONFIG_HOME is already set, it points the
// engine at the development configuration directory:
//
//   macOS / Linux: ~/.config/wspace-dev
//   Windows:       %APPDATA%\wspace-dev
//
// Installed and released binaries never go through this wrapper and keep
// the engine's own default (~/.config/wspace). Node is used instead of a
// shell snippet so the script behaves the same on every platform.
import { spawnSync } from "node:child_process";
import { homedir } from "node:os";
import { join } from "node:path";

export function devConfigHome(env = process.env, platform = process.platform, home = homedir()) {
  if (env.WSPACE_CONFIG_HOME) return env.WSPACE_CONFIG_HOME;
  if (platform === "win32") return join(env.APPDATA || home, "wspace-dev");
  return join(home, ".config", "wspace-dev");
}

const env = { ...process.env, WSPACE_CONFIG_HOME: devConfigHome() };
const result = spawnSync("go", ["run", "./cmd/wspace", ...process.argv.slice(2)], { stdio: "inherit", env });
if (result.error) {
  console.error(`wspace dev wrapper: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 1);
