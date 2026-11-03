import { spawnSync } from "node:child_process";
import { expect, test } from "vitest";

// Runs the real compiler. tsc stops before type errors when a file has mapper
// errors, so each kind of failure needs its own project.
const tsc = (project: string) => {
  const { stdout } = spawnSync(
    "node_modules/.bin/tsc",
    ["--noEmit", "--runExternalCode", "-p", `tests/testdata/${project}`, "--pretty", "false"],
    { encoding: "utf8" },
  );
  return stdout.trim().split("\n").map((line) => line.replace(`tests/testdata/${project}/`, ""));
};

test("TypeScript that fell behind the Go file is a type error", () => {
  expect(tsc("drift")).toEqual([
    expect.stringMatching(/^use\.ts\(4,\d+\): error TS\d+: Property 'totalPrice' does not exist/),
  ]);
}, 60_000);

test("a type JSON cannot carry is an error at its line in the Go file", () => {
  // Line and column stay right after the Japanese comment at the top of the file.
  expect(tsc("unsupported")).toEqual([
    expect.stringMatching(/^shared\.go\(5,6\): error go9201: 'int64' can exceed/),
    expect.stringMatching(/^shared\.go\(8,21\): error go9201: This type has no JSON form/),
    expect.stringMatching(/^shared\.go\(16,16\): error go9201: 'Money' customizes its encoding/),
  ]);
}, 60_000);
