// Starts a Go Wasm module and returns a function that looks up its exports.
// wasm_exec.js must already have defined globalThis.Go.
export async function loadGoModule(bytes, key) {
  const go = new globalThis.Go();
  const ready = new Promise((resolve) => {
    globalThis[key] = resolve;
  });
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  // Settles only if Go exits: once ready, it blocks forever to keep its functions callable.
  const exited = go.run(instance).then(() => {
    throw new Error(`The Go module ${key} exited during startup. Its panic message is in the console.`);
  });
  let exports;
  try {
    exports = await Promise.race([ready, exited]);
  } finally {
    delete globalThis[key];
  }

  return (name) => {
    const fn = exports[name];
    if (!fn) throw new Error(`The Go module ${key} does not export ${name}.`);
    return (...args) => {
      const { value, error } = fn(...args.map((arg) => JSON.stringify(arg) ?? "null"));
      if (error !== undefined) throw new Error(error);
      return value === "" ? undefined : JSON.parse(value);
    };
  };
}
