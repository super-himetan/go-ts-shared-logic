import { readFileSync } from "node:fs";
import { expect, test } from "vitest";
import { Menu, type Order, Price, type Quote, SizeL, TaxPercent, Validate } from "../shared/checkout/checkout.go";

type Case = { name: string; order: Order; quote?: Quote; violationPaths: string[] };

// The same file runs natively in shared/checkout/checkout_test.go.
const cases: Case[] = JSON.parse(
  readFileSync(new URL("../shared/checkout/testdata/cases.json", import.meta.url), "utf8"),
);

test.each(cases)("$name", ({ order, quote, violationPaths }) => {
  expect((Validate(order) ?? []).map((violation) => violation.path)).toEqual(violationPaths);
  if (quote) {
    expect(Price(order)).toEqual(quote);
  } else {
    expect(() => Price(order)).toThrow("金額を計算できません");
  }
});

test("Go consts are plain values", () => {
  expect(SizeL).toBe("L");
  expect(TaxPercent).toBe(10);
});

test("non-ASCII strings survive the bridge", () => {
  expect(Menu()?.[0]?.name).toBe("コーヒー");
});

test("a wrong call throws instead of killing the Go runtime", () => {
  // @ts-expect-error
  expect(() => Price()).toThrow("expected 1 argument(s), got 0");
  // @ts-expect-error
  expect(() => Price({ items: "nope" })).toThrow("argument 0");
  expect(Validate({ items: [] })).toHaveLength(1);
});

test.each([1.5, 1e30])("a number Go's int cannot hold throws: %s", (quantity) => {
  expect(() => Validate({ items: [{ drink: "coffee", size: "S", quantity }] })).toThrow("argument 0");
});
