package shapes

import (
	"errors"
	"time"
)

var last string

// One function for each shape the generated wrappers have to handle.

func Remember(a, b string) {
	last = a + b
}

func Recall() string {
	return last
}

func Forget() {
	last = ""
}

func Add(a, b int) int {
	return a + b
}

func Divide(a int, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

func Check(ok bool) error {
	if !ok {
		return errors.New("not ok")
	}
	return nil
}

func Fails() error {
	return errors.New("always")
}

func Panics() int {
	panic("kaboom")
}

// Sleep blocks, which only the standard Go compiler allows in a call from JavaScript.
func Sleep() int {
	time.Sleep(time.Millisecond)
	return 1
}
