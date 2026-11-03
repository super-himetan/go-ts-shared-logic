// Starts a Go Wasm module and returns a function that looks up its exports.
// Each module gets its own Go runtime. wasm_exec.js must already have defined globalThis.Go.
export async function loadGoModule(bytes, key) {
  const go = new globalThis.Go();
  const ready = new Promise((resolve) => {
    globalThis[key] = resolve;
  });
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  // Never settles: the Go side blocks forever to keep its functions callable.
  go.run(instance);
  const exports = await ready;
  delete globalThis[key];

  return (name) =>
    (...args) => {
      const { value, error } = exports[name](...args.map((arg) => JSON.stringify(arg) ?? "null"));
      if (error !== undefined) throw new Error(error);
      return value === "" ? undefined : JSON.parse(value);
    };
}
