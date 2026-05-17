import { cp, mkdir, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const desktopRoot = resolve(scriptDir, "..");
const repoRoot = resolve(desktopRoot, "..", "..");
const clientDir = resolve(repoRoot, "client");
const clientNodeModules = resolve(clientDir, "node_modules");
const sourceDist = resolve(clientDir, "dist");
const targetDist = resolve(desktopRoot, "frontend", "dist");
const shouldBuildClient = process.argv.includes("--build-client");

function run(command, args, options = {}) {
  const isWindows = process.platform === "win32";
  const result = isWindows
    ? spawnSync(`${command} ${args.join(" ")}`, {
        cwd: options.cwd,
        env: process.env,
        shell: true,
        stdio: "inherit",
      })
    : spawnSync(command, args, {
        cwd: options.cwd,
        env: process.env,
        stdio: "inherit",
      });

  if (result.error) {
    throw result.error;
  }

  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed.`);
  }
}

if (shouldBuildClient) {
  if (!existsSync(clientNodeModules)) {
    run("npm", ["install"], { cwd: clientDir });
  }

  run("npm", ["run", "build"], { cwd: clientDir });
}

if (!existsSync(sourceDist)) {
  throw new Error(
    `Client build output not found at ${sourceDist}. Run npm run build in client first.`,
  );
}

await rm(targetDist, { force: true, recursive: true });
await mkdir(targetDist, { recursive: true });
await cp(sourceDist, targetDist, { recursive: true });

console.log(`Synced ${sourceDist} to ${targetDist}`);
