// Package bridge calls Go functions with JSON arguments, which is how a
// function exported from a .go file is reached from JavaScript.
//
// It avoids calling functions through reflect, which TinyGo does not implement.
// The Vite plugin generates a typed wrapper for each function instead.
package bridge

import (
	"encoding/json"
	"fmt"
)

// Func is a wrapped function. It takes JSON arguments and returns its result as
// JSON, or "" if it has no value to return.
type Func = func(args []string) (string, error)

// CheckArgs reports a call with the wrong number of arguments.
func CheckArgs(args []string, want int) error {
	if len(args) != want {
		return fmt.Errorf("expected %d argument(s), got %d", want, len(args))
	}
	return nil
}

// Arg decodes the argument at index i.
func Arg[A any](args []string, i int) (a A, err error) {
	if err := json.Unmarshal([]byte(args[i]), &a); err != nil {
		return a, fmt.Errorf("argument %d: %w", i, err)
	}
	return a, nil
}

// Encode writes a function's result as JSON.
func Encode(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// Invoke calls fn and turns a panic into an error, because one that reaches the
// Wasm runtime kills every later call.
func Invoke(fn Func, args []string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(args)
}
