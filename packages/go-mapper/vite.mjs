import { createHash } from "node:crypto";
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { cacheDir, extractSchema, go } from "./schema.mjs";

const WASM_EXEC = "\0go-mapper:wasm_exec";
const BRIDGE = "github.com/super-himetan/go-ts-shared-logic/bridge";

function buildWasm(file, schema) {
  const dir = dirname(file);
  const importPath = go(["list", "-f", "{{.ImportPath}}", "."], { cwd: dir }).trim();
  const hash = createHash("sha256").update(importPath).digest("hex").slice(0, 12);
  const key = `__go_mapper_${hash}`;
  const entries = schema.funcs.map(({ name }) => `\t\t${JSON.stringify(name)}: pkg.${name},\n`).join("");

  const workDir = join(cacheDir, hash);
  mkdirSync(workDir, { recursive: true });
  const main = join(workDir, "main.go");
  writeFileSync(
    main,
    `package main\n\nimport (\n` +
      `\t"${BRIDGE}"\n\tpkg "${importPath}"\n)\n\n` +
      `func main() {\n\tbridge.Serve(${JSON.stringify(key)}, map[string]any{\n${entries}\t})\n}\n`,
  );
  const wasm = join(workDir, `${schema.package}.wasm`);
  go(["build", "-o", wasm, main], { cwd: dir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" } });
  return { wasm, key };
}

// Runtime side of `import "./x.go"`: compiles the file's package to Wasm and
// exports its functions. The content mapper only supplies types, which tsc erases.
export function goWasm() {
  let command;

  return {
    name: "go-wasm",
    enforce: "pre",
    configResolved(config) {
      command = config.command;
    },
    resolveId(id) {
      return id === WASM_EXEC ? id : undefined;
    },
    load(id) {
      if (id !== WASM_EXEC) return;
      const goroot = go(["env", "GOROOT"]).trim();
      return readFileSync(join(goroot, "lib/wasm/wasm_exec.js"), "utf8");
    },
    transform(code, id, options) {
      const file = id.split("?")[0];
      if (!file.endsWith(".go")) return;

      const schema = extractSchema(code);
      for (const diagnostic of schema.diagnostics) {
        const message = `go${diagnostic.code}: ${diagnostic.messageText}`;
        if (command === "build") this.error(message, diagnostic.start);
        else this.warn(message, diagnostic.start);
      }
      // The Wasm module holds the whole package, not only this file.
      for (const sibling of readdirSync(dirname(file))) {
        if (sibling.endsWith(".go")) this.addWatchFile(join(dirname(file), sibling));
      }

      const { wasm, key } = buildWasm(file, schema);
      const load = options?.ssr
        ? `import { readFile } from "node:fs/promises";\n` +
          `const bytes = await readFile(${JSON.stringify(wasm)});\n`
        : `import url from ${JSON.stringify(`/@fs${wasm}?url`)};\n` +
          `const bytes = await (await fetch(url)).arrayBuffer();\n`;
      const consts = schema.consts.map(({ name, value }) => `export const ${name} = ${value};\n`);
      const funcs = schema.funcs.map(({ name }) => `export const ${name} = lookup(${JSON.stringify(name)});\n`);
      return {
        code:
          `import ${JSON.stringify(WASM_EXEC)};\n` +
          `import { loadGoModule } from "go-mapper/runtime";\n` +
          load +
          `const lookup = await loadGoModule(bytes, ${JSON.stringify(key)});\n` +
          consts.join("") +
          funcs.join(""),
        map: null,
      };
    },
  };
}
