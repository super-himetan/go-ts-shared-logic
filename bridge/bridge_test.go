package bridge

import (
	"errors"
	"strings"
	"testing"
)

func TestInvoke(t *testing.T) {
	type pair struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	add := func(p pair) int { return p.A + p.B }
	fail := func() (int, error) { return 0, errors.New("boom") }
	onlyError := func() error { return nil }
	explode := func() int { panic("kaboom") }
	nothing := func() {}

	tests := []struct {
		name    string
		fn      any
		args    []string
		want    string
		wantErr string
	}{
		{name: "value", fn: add, args: []string{`{"a":1,"b":2}`}, want: "3"},
		{name: "error", fn: fail, wantErr: "boom"},
		{name: "nil error", fn: onlyError, want: ""},
		{name: "no result", fn: nothing, want: ""},
		{name: "panic", fn: explode, wantErr: "panic: kaboom"},
		{name: "argument count", fn: add, wantErr: "expected 1 argument(s), got 0"},
		{name: "bad json", fn: add, args: []string{"{"}, wantErr: "argument 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Invoke(tt.fn, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
