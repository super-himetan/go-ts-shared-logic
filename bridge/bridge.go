// Package bridge calls Go functions with JSON arguments, which is how a
// function exported from a .go file is reached from JavaScript.
package bridge

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Invoke calls fn with arguments decoded from JSON and returns its result as
// JSON. A function without a value to return yields "". A panic becomes an
// error, because one that reaches the Wasm runtime kills every later call.
func Invoke(fn any, args []string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	v := reflect.ValueOf(fn)
	t := v.Type()
	if len(args) != t.NumIn() {
		return "", fmt.Errorf("expected %d argument(s), got %d", t.NumIn(), len(args))
	}
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		p := reflect.New(t.In(i))
		if err := json.Unmarshal([]byte(arg), p.Interface()); err != nil {
			return "", fmt.Errorf("argument %d: %w", i, err)
		}
		in[i] = p.Elem()
	}

	out := v.Call(in)
	if n := len(out); n > 0 && t.Out(n-1) == reflect.TypeFor[error]() {
		if e, _ := out[n-1].Interface().(error); e != nil {
			return "", e
		}
		out = out[:n-1]
	}
	if len(out) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(out[0].Interface())
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
