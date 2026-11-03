// JSON writes these as null, which Go would read as a silent zero.
const encode = (arg) =>
  JSON.stringify(arg, (_, value) => {
    if (typeof value === "number" && !Number.isFinite(value)) throw new Error(`Cannot send ${value} to Go.`);
    return value;
  }) ?? "null";

// Starts a Go Wasm module and returns a function that looks up its exports.
// wasm_exec.js must already have defined globalThis.Go.
export async function loadGoModule(bytes, key) {
  const go = new globalThis.Go();
  const ready = new Promise((resolve) => {
    globalThis[key] = resolve;
  });
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  // Settles only if Go exits: once ready, it blocks forever to keep its functions callable.
  const failed = (cause) => {
    throw new Error(`The Go module ${key} exited during startup. Any panic message is in the console.`, { cause });
  };
  const exited = go.run(instance).then(failed, failed);
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
      const result = fn(...args.map(encode));
      // TinyGo cannot wait inside a call from JavaScript, and returns nothing if the function tries.
      if (result === undefined) throw new Error(`${name} blocked, which TinyGo does not allow in a call from JavaScript.`);
      const { value, error } = result;
      if (error !== undefined) throw new Error(error);
      return value === "" ? undefined : JSON.parse(value);
    };
  };
}
