import {
  type Drink,
  type ErrorResponse,
  type LineItem,
  MaxQuantityPerLine,
  Menu,
  type Order,
  type PlaceOrderRequest,
  type PlaceOrderResponse,
  Price,
  type Quote,
  type Size,
  SizeL,
  SizeM,
  SizeS,
  TaxPercent,
  Validate,
} from "../shared/checkout/checkout.go";

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const yen = (amount: number) => `${amount.toLocaleString("ja-JP")}円`;
const menu = Menu() ?? [];
const sizes: Size[] = [SizeS, SizeM, SizeL];

let items: LineItem[] = [{ drink: "coffee", size: SizeM, quantity: 1 }];

const currentOrder = (): Order => {
  const coupon = $<HTMLInputElement>("coupon").value.trim();
  return coupon ? { items, coupon } : { items };
};

// Everything below this line is computed by the Go code, without a request.
function refresh() {
  const order = currentOrder();
  const violations = Validate(order) ?? [];
  $("violations").replaceChildren(
    ...violations.map(({ message }) => Object.assign(document.createElement("li"), { textContent: message })),
  );
  const valid = violations.length === 0;
  $<HTMLButtonElement>("submit").disabled = !valid;
  $<HTMLButtonElement>("tamper").disabled = !valid;
  renderQuote(valid ? Price(order) : undefined);
}

function renderQuote(quote: Quote | undefined) {
  const rows: [string, string][] = [
    ["小計", quote ? yen(quote.subtotal) : "-"],
    ["割引", quote ? yen(0 - quote.discount) : "-"],
    [`消費税 (${TaxPercent}%)`, quote ? yen(quote.tax) : "-"],
    ["合計", quote ? yen(quote.total) : "-"],
  ];
  $("quote").replaceChildren(
    ...rows.map(([label, value]) => {
      const row = document.createElement("tr");
      row.append(
        Object.assign(document.createElement("th"), { textContent: label }),
        Object.assign(document.createElement("td"), { textContent: value }),
      );
      return row;
    }),
  );
}

function renderItems() {
  $("items").replaceChildren(
    ...items.map((item, index) => {
      const row = document.createElement("div");
      row.className = "item";

      const drink = document.createElement("select");
      drink.setAttribute("aria-label", "商品");
      drink.append(...menu.map((entry) => new Option(`${entry.name} ${yen(entry.price)}`, entry.drink)));
      drink.value = item.drink;
      drink.onchange = () => update(index, { drink: drink.value as Drink });

      const size = document.createElement("select");
      size.setAttribute("aria-label", "サイズ");
      size.append(...sizes.map((value) => new Option(value, value)));
      size.value = item.size;
      size.onchange = () => update(index, { size: size.value as Size });

      const quantity = document.createElement("input");
      quantity.type = "number";
      quantity.setAttribute("aria-label", "数量");
      quantity.max = String(MaxQuantityPerLine);
      quantity.value = String(item.quantity);
      // Go's int rejects anything else, so let Validate report it as out of range.
      quantity.oninput = () => {
        const value = quantity.valueAsNumber;
        update(index, { quantity: Number.isSafeInteger(value) ? value : 0 });
      };

      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "secondary";
      remove.textContent = "削除";
      remove.onclick = () => {
        items = items.filter((_, i) => i !== index);
        renderItems();
        refresh();
      };

      row.append(drink, size, quantity, remove);
      return row;
    }),
  );
}

function update(index: number, patch: Partial<LineItem>) {
  items = items.map((item, i) => (i === index ? { ...item, ...patch } : item));
  refresh();
}

async function placeOrder(expectedTotal: number) {
  const order = currentOrder();
  const body: PlaceOrderRequest = { order, expectedTotal };
  const result = $("result");
  try {
    const response = await fetch("/api/orders", { method: "POST", body: JSON.stringify(body) });
    if (response.ok) {
      const { orderId, quote }: PlaceOrderResponse = await response.json();
      result.textContent = `受付: ${orderId} / サーバーが計算した合計 ${yen(quote.total)}`;
    } else {
      const { message }: ErrorResponse = await response.json();
      result.textContent = `拒否 (${response.status}): ${message}`;
    }
  } catch {
    result.textContent = "サーバーに接続できません。pnpm server を起動してください。";
  }
}

$("add-item").onclick = () => {
  items = [...items, { drink: "latte", size: SizeM, quantity: 1 }];
  renderItems();
  refresh();
};
$("coupon").oninput = refresh;
$("order").onsubmit = (event) => {
  event.preventDefault();
  void placeOrder(Price(currentOrder()).total);
};
$("tamper").onclick = () => void placeOrder(Price(currentOrder()).total - 100);

renderItems();
refresh();
