// Runs tools/goschema, the one parser behind both the types and the runtime module.
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "../..");
export const cacheDir = join(repoRoot, "node_modules/.cache/go-mapper");

export function go(args, options = {}) {
  const result = spawnSync("go", args, { cwd: repoRoot, encoding: "utf8", ...options });
  if (result.error) throw new Error(`Could not run go: ${result.error.message}`);
  if (result.status !== 0) throw new Error(`go ${args.join(" ")}\n${result.stderr}`);
  return result.stdout;
}

// Built once per process, because `go run` on every keystroke would be too slow.
let binary;

export function extractSchema(source) {
  if (!binary) {
    const path = join(cacheDir, "goschema");
    go(["build", "-o", path, "./tools/goschema"]);
    binary = path;
  }
  const result = spawnSync(binary, { input: source, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`goschema: ${result.stderr}`);
  return JSON.parse(result.stdout);
}
