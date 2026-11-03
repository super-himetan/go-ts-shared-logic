import type { Result } from "./shared.go";

// The Go tag says `total`, and this still uses the old name.
export const read = (result: Result) => result.totalPrice;
