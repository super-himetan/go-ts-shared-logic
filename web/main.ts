// A dynamic import, because a static one would stop this file from running at
// all when the Wasm module fails to load, leaving nothing to report it.
try {
  const { start } = await import("./app");
  start();
} catch (error) {
  console.error(error);
  document.getElementById("result")!.textContent = "読み込みに失敗しました。ページを再読み込みしてください。";
}

export {};
