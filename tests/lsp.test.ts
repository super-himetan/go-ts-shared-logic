import { type ChildProcess, spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { afterAll, beforeAll, expect, test } from "vitest";

// What an editor gets: the language server started the way the VS Code extension starts it.
const uri = (path: string) => pathToFileURL(resolve(path)).href;

let server: ChildProcess;
let nextId = 1;
const pending = new Map<number, (result: any) => void>();

const send = (message: object) => {
  const body = JSON.stringify({ jsonrpc: "2.0", ...message });
  server.stdin!.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
};
const request = (method: string, params: object) =>
  new Promise<any>((done) => {
    const id = nextId++;
    pending.set(id, done);
    send({ id, method, params });
  });
const open = (path: string, languageId: string) =>
  send({
    method: "textDocument/didOpen",
    params: { textDocument: { uri: uri(path), languageId, version: 1, text: readFileSync(path, "utf8") } },
  });
const at = (path: string, needle: string, offset = 0) => {
  const lines = readFileSync(path, "utf8").split("\n");
  const line = lines.findIndex((text) => text.includes(needle));
  return { textDocument: { uri: uri(path) }, position: { line, character: lines[line]!.indexOf(needle) + offset } };
};

const APP = "web/app.ts";
const CHECKOUT = "shared/checkout/checkout.go";

beforeAll(async () => {
  server = spawn("node_modules/.bin/tsc", ["--lsp", "--stdio"]);
  let buffer = Buffer.alloc(0);
  server.stdout!.on("data", (chunk: Buffer) => {
    buffer = Buffer.concat([buffer, chunk]);
    for (;;) {
      const end = buffer.indexOf("\r\n\r\n");
      if (end < 0) return;
      const length = Number(/Content-Length: (\d+)/i.exec(buffer.subarray(0, end).toString())![1]);
      if (buffer.length < end + 4 + length) return;
      const message = JSON.parse(buffer.subarray(end + 4, end + 4 + length).toString());
      buffer = buffer.subarray(end + 4 + length);
      if (message.method && message.id !== undefined) {
        const result = message.method === "workspace/configuration" ? message.params.items.map(() => ({})) : null;
        send({ id: message.id, result });
      } else if (message.id !== undefined) {
        pending.get(message.id)?.(message.result);
        pending.delete(message.id);
      }
    }
  });
  await request("initialize", {
    processId: process.pid,
    rootUri: uri("."),
    initializationOptions: { runExternalCode: true },
    capabilities: { textDocument: { hover: { contentFormat: ["markdown"] }, diagnostic: {} } },
  });
  send({ method: "initialized", params: {} });
  open(APP, "typescript");
}, 60_000);

afterAll(() => {
  server.kill();
});

test("hovering a Go function shows its signature and Go comment", async () => {
  const hover = await request("textDocument/hover", at(APP, "  Price,", 2));
  expect(hover.contents.value).toContain("function Price(order: Order): Quote");
  expect(hover.contents.value).toContain("Price computes the amount to pay.");
}, 60_000);

test("go to definition jumps into the Go file", async () => {
  const [definition] = await request("textDocument/definition", at(APP, "  Price,", 2));
  expect(definition.uri).toBe(uri(CHECKOUT));
  expect(definition.range.start).toEqual(at(CHECKOUT, "func Price(", 5).position);
}, 60_000);

test("a property leads to its JSON tag, and renaming it edits the tag", async () => {
  const tag = at(CHECKOUT, 'json:"subtotal"', 6).position;
  const property = at(APP, "quote.subtotal", 6);
  const [definition] = await request("textDocument/definition", property);
  expect(definition.range.start).toEqual(tag);

  const rename = await request("textDocument/rename", { ...property, newName: "sub" });
  expect(rename.changes[uri(CHECKOUT)]).toEqual([expect.objectContaining({ newText: "sub" })]);
  expect(rename.changes[uri(CHECKOUT)][0].range.start).toEqual(tag);
}, 60_000);

test("a Go function cannot be renamed from TypeScript, which would miss its Go callers", async () => {
  const rename = await request("textDocument/rename", { ...at(APP, "  Price,", 2), newName: "Cost" });
  expect(rename?.changes?.[uri(CHECKOUT)]).toBeUndefined();
}, 60_000);

test("problems are reported on the Go file", async () => {
  const path = "tests/testdata/unsupported/shared.go";
  open(path, "go");
  const { items } = await request("textDocument/diagnostic", { textDocument: { uri: uri(path) } });
  expect(items.map((item: { source: string; code: number }) => `${item.source}${item.code}`)).toEqual([
    "go9201",
    "go9201",
    "go9201",
  ]);
  expect(items[0].range.start).toEqual({ line: 4, character: 5 });
}, 60_000);
