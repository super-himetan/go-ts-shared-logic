import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { cacheDir, extractSchema, go } from "./schema.mjs";

const WASM_EXEC = "\0go-mapper:wasm_exec";
const PACKAGE = "\0go-mapper:package:";
const BRIDGE = "github.com/super-himetan/go-ts-shared-logic/bridge";

// One Wasm module per package, holding the functions of all its files, so
// those files share a Go runtime just as they do in a native build.
function buildWasm(dir) {
  const env = { ...process.env, GOOS: "js", GOARCH: "wasm" };
  const pkg = JSON.parse(go(["list", "-json=ImportPath,Name,GoFiles", "."], { cwd: dir, env }));
  const files = pkg.GoFiles.map((name) => join(dir, name));
  const funcs = files.flatMap((file) => extractSchema(readFileSync(file, "utf8")).funcs);

  const hash = createHash("sha256").update(pkg.ImportPath).digest("hex").slice(0, 12);
  const key = `__go_mapper_${hash}`;
  const entries = funcs.map(({ name }) => `\t\t${JSON.stringify(name)}: pkg.${name},\n`).join("");

  const workDir = join(cacheDir, hash);
  mkdirSync(workDir, { recursive: true });
  const main = join(workDir, "main.go");
  writeFileSync(
    main,
    `package main\n\nimport (\n` +
      `\t"${BRIDGE}"\n\tpkg "${pkg.ImportPath}"\n)\n\n` +
      `func main() {\n\tbridge.Serve(${JSON.stringify(key)}, map[string]any{\n${entries}\t})\n}\n`,
  );
  const wasm = join(workDir, `${pkg.Name}.wasm`);
  go(["build", "-o", wasm, main], { cwd: dir, env });
  return { wasm, key, files };
}

// Runtime side of `import "./x.go"`: exports the file's consts and functions.
// The content mapper only supplies types, which tsc erases.
export function goWasm() {
  let command;

  return {
    name: "go-wasm",
    enforce: "pre",
    configResolved(config) {
      command = config.command;
    },
    resolveId(id) {
      return id === WASM_EXEC || id.startsWith(PACKAGE) ? id : undefined;
    },
    load(id, options) {
      if (id === WASM_EXEC) {
        const goroot = go(["env", "GOROOT"]).trim();
        return readFileSync(join(goroot, "lib/wasm/wasm_exec.js"), "utf8");
      }
      if (!id.startsWith(PACKAGE)) return;

      const dir = Buffer.from(id.slice(PACKAGE.length), "base64url").toString();
      const { wasm, key, files } = buildWasm(dir);
      for (const file of files) this.addWatchFile(file);
      const bytes = options?.ssr
        ? `import { readFile } from "node:fs/promises";\n` +
          `const bytes = await readFile(${JSON.stringify(wasm)});\n`
        : `import url from ${JSON.stringify(`/@fs${wasm}?url`)};\n` +
          `const bytes = await (await fetch(url)).arrayBuffer();\n`;
      return (
        `import ${JSON.stringify(WASM_EXEC)};\n` +
        `import { loadGoModule } from "go-mapper/runtime";\n` +
        bytes +
        `export const lookup = await loadGoModule(bytes, ${JSON.stringify(key)});\n`
      );
    },
    transform(code, id) {
      const file = id.split("?")[0];
      if (!file.endsWith(".go")) return;

      const schema = extractSchema(code);
      for (const diagnostic of schema.diagnostics) {
        const message = `go${diagnostic.code}: ${diagnostic.messageText}`;
        if (command === "build") this.error(message, diagnostic.start);
        else this.warn(message, diagnostic.start);
      }

      const consts = schema.consts.map(({ name, value }) => `export const ${name} = ${value};\n`);
      const funcs = schema.funcs.map(({ name }) => `export const ${name} = lookup(${JSON.stringify(name)});\n`);
      // A file without functions has no reason to download the Go runtime.
      const runtime = funcs.length
        ? `import { lookup } from ${JSON.stringify(PACKAGE + Buffer.from(dirname(file)).toString("base64url"))};\n`
        : "";
      return { code: runtime + consts.join("") + funcs.join(""), map: null };
    },
  };
}
