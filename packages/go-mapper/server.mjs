import { MappedCodeBuilder, runContentMapper, SpanMapFeature } from "ts-content-mapper";
import { extractSchema } from "./schema.mjs";

// Renaming a Go identifier from TypeScript would miss its Go callers, so leave that to gopls.
const NO_RENAME = SpanMapFeature.All & ~SpanMapFeature.Rename;
const IDENTIFIER = /^[A-Za-z_$][\w$]*$/;

const writeDoc = (b, indent, doc, extra = []) => {
  const lines = [...(doc ? doc.split("\n") : []), ...extra];
  if (!lines.length) return;
  const safe = lines.map((line) => line.replaceAll("*/", "*\\/"));
  b.append(`${indent}/**\n${safe.map((line) => `${indent} * ${line}\n`).join("")}${indent} */\n`);
};

function writeTypes(schema, b) {
  for (const type of schema.types) {
    writeDoc(b, "", type.doc);
    b.append(type.fields ? "export interface " : "export type ");
    b.appendVerbatim(type.span.start, type.span.end, NO_RENAME);
    if (!type.fields) {
      b.append(` = ${type.alias};\n`);
      continue;
    }
    b.append(" {\n");
    for (const field of type.fields) {
      writeDoc(b, "  ", field.doc);
      b.append("  ");
      if (!IDENTIFIER.test(field.name)) {
        b.appendAlias(JSON.stringify(field.name), field.span.start, field.span.end, NO_RENAME);
      } else {
        // A tag only names the field on the wire, so renaming it is safe for Go.
        b.appendVerbatim(field.span.start, field.span.end, field.tagged ? SpanMapFeature.All : NO_RENAME);
      }
      b.append(`${field.optional ? "?" : ""}: ${field.ts};\n`);
    }
    b.append("}\n");
  }
  for (const constant of schema.consts) {
    writeDoc(b, "", constant.doc);
    b.append("export declare const ");
    b.appendVerbatim(constant.span.start, constant.span.end, NO_RENAME);
    b.append(`: ${constant.value};\n`);
  }
  for (const fn of schema.funcs) {
    writeDoc(b, "", fn.doc, fn.throws ? ["@throws {Error} The Go function returned an error."] : []);
    b.append("export declare function ");
    b.appendVerbatim(fn.span.start, fn.span.end, NO_RENAME);
    const params = fn.params.map(({ name, ts }) => `${name}: ${ts}`).join(", ");
    b.append(`(${params}): ${fn.result};\n`);
  }
  // A file with nothing to export must still be a module.
  b.append("export {};\n");
}

runContentMapper({
  diagnosticSource: "go",
  transform(params, context) {
    const b = new MappedCodeBuilder(params.content, { positionEncoding: context.positionEncoding });
    let schema;
    try {
      schema = extractSchema(params.content);
    } catch (error) {
      return {
        ...b.append("export {};\n").build(".ts"),
        diagnostics: [{ code: 9299, messageText: error.message, start: 0, length: 1 }],
      };
    }
    writeTypes(schema, b);
    return { ...b.build(".ts"), diagnostics: schema.diagnostics };
  },
});
