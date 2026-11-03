import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { cacheDir, extractSchema, go, repoRoot, run } from "./schema.mjs";

const WASM_EXEC = "\0go-mapper:wasm_exec";
const PACKAGE = "\0go-mapper:package:";
const BRIDGE = "github.com/super-himetan/go-ts-shared-logic/bridge";

const COMPILERS = {
  tinygo: {
    build(wasm, main, cwd) {
      // TinyGo calls wasm-opt, which its releases do not always include.
      const env = { ...process.env, WASMOPT: join(repoRoot, "node_modules/.bin/wasm-opt") };
      run("tinygo", ["build", "-o", wasm, "-target", "wasm", "-opt=z", "-no-debug", main], { cwd, env });
    },
    wasmExec: () => join(run("tinygo", ["env", "TINYGOROOT"]).trim(), "targets/wasm_exec.js"),
  },
  go: {
    build(wasm, main, cwd) {
      go(["build", "-o", wasm, main], { cwd, env: { ...process.env, GOOS: "js", GOARCH: "wasm" } });
    },
    wasmExec: () => join(go(["env", "GOROOT"]).trim(), "lib/wasm/wasm_exec.js"),
  },
};

const shapeOf = (fn) => ({ arity: fn.params.length, hasResult: fn.result !== "void", throws: fn.throws });
const wrapperName = ({ arity, hasResult, throws }) => `wrap${arity}${hasResult ? "R" : ""}${throws ? "E" : ""}`;

// A generic function, so that Go infers the types and nothing here has to name
// them. Each shape of function that the package uses gets one.
function writeWrapper(shape) {
  const { arity, hasResult, throws } = shape;
  const params = Array.from({ length: arity }, (_, i) => `A${i}`);
  const typeParams = [...params, ...(hasResult ? ["R"] : [])];
  const generics = typeParams.length ? `[${typeParams.join(", ")} any]` : "";
  const results = [...(hasResult ? ["R"] : []), ...(throws ? ["error"] : [])];
  const signature = results.length > 1 ? `(${results.join(", ")})` : (results[0] ?? "");
  const call = `f(${params.map((_, i) => `a${i}`).join(", ")})`;

  const decode = params.map(
    (param, i) =>
      `\t\ta${i}, err := bridge.Arg[${param}](args, ${i})\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n`,
  );
  let finish;
  if (hasResult && throws) {
    finish = `\t\tr, err := ${call}\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\treturn bridge.Encode(r)\n`;
  } else if (hasResult) {
    finish = `\t\treturn bridge.Encode(${call})\n`;
  } else if (throws) {
    finish = `\t\treturn "", ${call}\n`;
  } else {
    finish = `\t\t${call}\n\t\treturn "", nil\n`;
  }
  return (
    `func ${wrapperName(shape)}${generics}(f func(${params.join(", ")}) ${signature}) bridge.Func {\n` +
    `\treturn func(args []string) (string, error) {\n` +
    `\t\tif err := bridge.CheckArgs(args, ${arity}); err != nil {\n\t\t\treturn "", err\n\t\t}\n` +
    decode.join("") +
    finish +
    `\t}\n}\n\n`
  );
}

// One Wasm module per package, holding the functions of all its files, so
// those files share a Go runtime just as they do in a native build.
function buildWasm(dir, compiler) {
  const env = { ...process.env, GOOS: "js", GOARCH: "wasm" };
  const pkg = JSON.parse(go(["list", "-json=ImportPath,Name,GoFiles", "."], { cwd: dir, env }));
  const files = pkg.GoFiles.map((name) => join(dir, name));
  const funcs = files.flatMap((file) => extractSchema(readFileSync(file, "utf8")).funcs);

  const hash = createHash("sha256").update(pkg.ImportPath).digest("hex").slice(0, 12);
  const key = `__go_mapper_${pkg.Name}_${hash}`;
  const shapes = new Map(funcs.map((fn) => [wrapperName(shapeOf(fn)), shapeOf(fn)]));
  const entries = funcs.map((fn) => `\t\t${JSON.stringify(fn.name)}: ${wrapperName(shapeOf(fn))}(pkg.${fn.name}),\n`);

  const workDir = join(cacheDir, compiler, hash);
  mkdirSync(workDir, { recursive: true });
  const main = join(workDir, "main.go");
  writeFileSync(
    main,
    `package main\n\nimport (\n` +
      `\t"${BRIDGE}"\n\tpkg "${pkg.ImportPath}"\n)\n\n` +
      [...shapes.values()].map(writeWrapper).join("") +
      `func main() {\n\tbridge.Serve(${JSON.stringify(key)}, map[string]bridge.Func{\n${entries.join("")}\t})\n}\n`,
  );
  const wasm = join(workDir, `${pkg.Name}.wasm`);
  COMPILERS[compiler].build(wasm, main, dir);
  return { wasm, key, files };
}

// Runtime side of `import "./x.go"`: exports the file's consts and functions.
// The content mapper only supplies types, which tsc erases.
//
// A build uses TinyGo for its size. Everything else uses the standard compiler,
// because TinyGo takes seconds where Go takes a fraction of one.
export function goWasm({ compiler: requested = process.env.GO_WASM_COMPILER } = {}) {
  if (requested && !COMPILERS[requested]) throw new Error(`Unknown Go compiler '${requested}'. Use 'tinygo' or 'go'.`);
  let command;
  let compiler;

  return {
    name: "go-wasm",
    enforce: "pre",
    configResolved(config) {
      command = config.command;
      compiler = requested ?? (command === "build" ? "tinygo" : "go");
    },
    resolveId(id) {
      return id === WASM_EXEC || id.startsWith(PACKAGE) ? id : undefined;
    },
    load(id, options) {
      if (id === WASM_EXEC) return readFileSync(COMPILERS[compiler].wasmExec(), "utf8");
      if (!id.startsWith(PACKAGE)) return;

      const dir = Buffer.from(id.slice(PACKAGE.length), "base64url").toString();
      const { wasm, key, files } = buildWasm(dir, compiler);
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
    // Too large to compress per request, and it only has to be done once per build.
    generateBundle(_, bundle) {
      for (const output of Object.values(bundle)) {
        if (output.type !== "asset" || !output.fileName.endsWith(".wasm")) continue;
        const { fileName, source } = output;
        this.emitFile({
          type: "asset",
          fileName: `${fileName}.br`,
          source: brotliCompressSync(source, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }),
        });
        this.emitFile({ type: "asset", fileName: `${fileName}.gz`, source: gzipSync(source, { level: 9 }) });
      }
    },
  };
}
