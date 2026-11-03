import { expect, test } from "@playwright/test";

const total = (page: import("@playwright/test").Page) => page.locator("#quote tr:last-child td");

test("the price is computed in the browser, without a request", async ({ page }) => {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/")) requests.push(request.url());
  });
  await page.goto("/");
  await expect(total(page)).toHaveText("473円");

  await page.getByLabel("数量").fill("3");
  await page.getByLabel("クーポン").fill("STAFF30");
  await expect(total(page)).toHaveText("993円");
  expect(requests).toEqual([]);
});

test("the Wasm arrives compressed", async ({ page }) => {
  const wasm = page.waitForResponse((response) => response.url().endsWith(".wasm"));
  await page.goto("/");
  const headers = (await wasm).headers();
  expect(headers["content-encoding"]).toBe("br");
  expect(headers["content-type"]).toBe("application/wasm");
});

test("the form stays disabled until the Go code has loaded", async ({ page }) => {
  let release = () => {};
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/*.wasm", async (route) => {
    await released;
    await route.continue();
  });
  await page.goto("/");

  await expect(page.getByRole("status")).toHaveText("読み込んでいます…");
  await expect(page.getByRole("button", { name: "注文する" })).toBeDisabled();

  release();
  await expect(page.getByRole("button", { name: "注文する" })).toBeEnabled();
  await expect(page.getByRole("status")).toHaveText("");
});

test("a failed load is reported", async ({ page }) => {
  await page.route("**/*.wasm", (route) => route.fulfill({ status: 500, body: "broken" }));
  await page.goto("/");
  await expect(page.getByRole("status")).toContainText("読み込みに失敗しました");
  await expect(page.getByRole("button", { name: "注文する" })).toBeDisabled();
});

test("a double click places one order", async ({ page }) => {
  let posts = 0;
  await page.route("**/api/orders", async (route) => {
    posts++;
    await new Promise((resolve) => setTimeout(resolve, 300));
    await route.continue();
  });
  await page.goto("/");
  const submit = page.getByRole("button", { name: "注文する" });
  await expect(submit).toBeEnabled();

  await submit.dblclick();
  await expect(page.getByRole("status")).toContainText("受付:");
  await expect(submit).toBeEnabled();
  expect(posts).toBe(1);

  await page.getByLabel("数量").fill("2");
  await expect(page.getByRole("status")).toHaveText("");
});

test("the server recomputes the total and refuses a lower one", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "表示より安い金額で送る" }).click();
  await expect(page.getByRole("status")).toHaveText("拒否 (409): 金額が変わりました: 表示 373円 / 実際 473円");
});

test("a value Go's int cannot hold is shown as a violation", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("数量").fill("1.5");
  await expect(page.locator("#violations")).toHaveText("1行目: 数量は1〜9で指定してください");
  await expect(page.getByRole("button", { name: "注文する" })).toBeDisabled();
});

test("files of one package share a Go runtime in a production bundle", async ({ page }) => {
  await page.goto("http://localhost:5186/");
  await expect(page.locator("#result")).toHaveText("count=2 limit=3");
});
