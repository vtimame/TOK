#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const paths = ["web/openapi.json", "web/src/api/generated"];
const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const webRoot = path.join(repoRoot, "web");

function run(command, args, cwd = repoRoot) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    stdio: "inherit",
  });

  if (result.error) {
    console.error(`failed to run ${command}: ${result.error.message}`);
    process.exit(1);
  }

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function scopedStatus() {
  const result = spawnSync("git", ["status", "--porcelain", "--", ...paths], {
    cwd: repoRoot,
    encoding: "utf8",
  });

  if (result.error) {
    console.error(`failed to run git status: ${result.error.message}`);
    process.exit(1);
  }

  if (result.status !== 0) {
    process.stderr.write(result.stderr);
    process.exit(result.status ?? 1);
  }

  return result.stdout.trim();
}

const before = scopedStatus();

run("pnpm", ["api:validate"], webRoot);
run("pnpm", ["api:generate"], webRoot);

const after = scopedStatus();
if (after === before) {
  console.log("OpenAPI schema and generated client are up to date.");
  process.exit(0);
}

console.error("OpenAPI schema or generated client drift detected after regeneration:");
console.error(after || "(no scoped git status)");
console.error("");
console.error("Run `cd web && pnpm api:generate`, review the diff, and commit the generated changes.");
process.exit(1);
