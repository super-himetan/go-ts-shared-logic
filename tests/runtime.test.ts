import { expect, test } from "vitest";

// Dynamic imports, so one module failing to load only fails its own test.
const timeout = 60_000;

test("files of one package share a Go runtime", async () => {
  const { Bump } = await import("./testdata/multi/x.go");
  const { Count } = await import("./testdata/multi/y.go");
  Bump();
  expect(Count()).toBe(1);
}, timeout);

test("every shape of function is callable", async () => {
  const { Add, Check, Divide, Fails, Forget, Panics, Recall, Remember } = await import("./testdata/shapes/shapes.go");
  expect(Add(2, 3)).toBe(5);
  expect(Divide(6, 3)).toBe(2);
  expect(() => Divide(1, 0)).toThrow("division by zero");
  expect(Check(true)).toBeUndefined();
  expect(() => Check(false)).toThrow("not ok");
  expect(() => Fails()).toThrow("always");

  expect(Remember("a", "b")).toBeUndefined();
  expect(Recall()).toBe("ab");
  expect(Forget()).toBeUndefined();
  expect(Recall()).toBe("");

  // A panic does not take the runtime down with it.
  expect(() => Panics()).toThrow("panic: kaboom");
  // @ts-expect-error
  expect(() => Add(1)).toThrow("expected 2 argument(s), got 1");
  expect(Add(1, 1)).toBe(2);
}, timeout);

test("a function that blocks is the one difference between the compilers", async () => {
  const { Sleep, Add } = await import("./testdata/shapes/shapes.go");
  if (process.env.GO_WASM_COMPILER === "tinygo") {
    expect(() => Sleep()).toThrow("Sleep blocked");
  } else {
    expect(Sleep()).toBe(1);
  }
  expect(Add(1, 1)).toBe(2);
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
