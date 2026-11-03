//go:build js && wasm

package bridge

import "syscall/js"

// Serve hands the functions to the JavaScript callback stored at
// globalThis[key], then keeps the Go runtime alive so they stay callable.
func Serve(key string, funcs map[string]Func) {
	exports := map[string]any{}
	for name, fn := range funcs {
		exports[name] = js.FuncOf(func(_ js.Value, args []js.Value) any {
			encoded := make([]string, len(args))
			for i, arg := range args {
				encoded[i] = arg.String()
			}
			result, err := Invoke(fn, encoded)
			if err != nil {
				return map[string]any{"error": err.Error()}
			}
			return map[string]any{"value": result}
		})
	}
	js.Global().Call(key, exports)
	select {}
}
