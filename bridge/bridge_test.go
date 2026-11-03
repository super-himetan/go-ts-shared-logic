package bridge

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckArgs(t *testing.T) {
	if err := CheckArgs([]string{"1"}, 1); err != nil {
		t.Error(err)
	}
	err := CheckArgs(nil, 1)
	if err == nil || err.Error() != "expected 1 argument(s), got 0" {
		t.Errorf("err = %v", err)
	}
}

func TestArg(t *testing.T) {
	type pair struct {
		A int `json:"a"`
	}
	got, err := Arg[pair]([]string{"0", `{"a":1}`}, 1)
	if err != nil || got.A != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
	if _, err := Arg[pair]([]string{"{"}, 0); err == nil || !strings.Contains(err.Error(), "argument 0") {
		t.Errorf("err = %v", err)
	}
	if _, err := Arg[int]([]string{"1.5"}, 0); err == nil {
		t.Error("1.5 was accepted as an int")
	}
}

func TestEncode(t *testing.T) {
	if got, err := Encode(map[string]int{"a": 1}); err != nil || got != `{"a":1}` {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := Encode(make(chan int)); err == nil {
		t.Error("a channel was encoded")
	}
}

func TestInvoke(t *testing.T) {
	tests := []struct {
		name    string
		fn      Func
		want    string
		wantErr string
	}{
		{name: "value", fn: func([]string) (string, error) { return "3", nil }, want: "3"},
		{name: "error", fn: func([]string) (string, error) { return "", errors.New("boom") }, wantErr: "boom"},
		{name: "panic", fn: func([]string) (string, error) { panic("kaboom") }, wantErr: "panic: kaboom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Invoke(tt.fn, nil)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %q, %v", got, err)
			}
		})
	}
}
