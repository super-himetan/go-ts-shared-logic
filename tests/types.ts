// Checked by `pnpm check`. Each @ts-expect-error fails the check if Go stops rejecting that code.
import { MaxQuantityPerLine, type Order, Price, type Quote, type Size, Validate } from "../shared/checkout/checkout.go";

const order: Order = { items: [{ drink: "coffee", size: "M", quantity: 1 }] };

const quote: Quote = Price(order);
const total: number = quote.total;
const limit: 9 = MaxQuantityPerLine;
const size: Size = "S";

// A nil slice is written as null, so the compiler makes the caller handle it.
const paths = (Validate(order) ?? []).map((violation) => violation.path);

// A value that is not one of the Go consts.
// @ts-expect-error
const badDrink: Order = { items: [{ drink: "tea", size: "M", quantity: 1 }] };

// `coupon` has omitempty, so it is optional, but `items` is not.
// @ts-expect-error
const noItems: Order = { coupon: "WELCOME10" };

// Names come from the JSON tags, not the Go fields.
// @ts-expect-error
const wrongName = quote.Total;

// A nil slice has to be handled before use.
// @ts-expect-error
const unhandled = Validate(order).length;

// @ts-expect-error
Price();

export { limit, paths, size, total, badDrink, noItems, wrongName, unhandled };
