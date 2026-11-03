import { expect, test } from "vitest";

// Dynamic imports, so one module failing to load only fails its own test.
const timeout = 60_000;

test("files of one package share a Go runtime", async () => {
  const { Bump } = await import("./testdata/multi/x.go");
  const { Count } = await import("./testdata/multi/y.go");
  Bump();
  expect(Count()).toBe(1);
}, timeout);

test("a parameter name TypeScript reserves still works", async () => {
  const { Echo } = await import("./testdata/multi/x.go");
  expect(Echo("x")).toBe("x");
}, timeout);

test("a file without functions can be imported", async () => {
  const { Limit } = await import("./testdata/constsonly/limits.go");
  expect(Limit).toBe(3);
}, timeout);

test("a panic during startup fails the import instead of hanging", async () => {
  await expect(import("./testdata/initpanic/panics.go")).rejects.toThrow("exited during startup");
}, timeout);
