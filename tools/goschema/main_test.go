package main

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func extract(t *testing.T, src string) Schema {
	t.Helper()
	return Extract([]byte(src))
}

func slice(src string, span Span) string {
	units := utf16.Encode([]rune(src))
	return string(utf16.Decode(units[span.Start:span.End]))
}

func TestStruct(t *testing.T) {
	src := "package p\n" +
		"// 日本語🍵のコメントがあっても位置がずれない\n" +
		"type Order struct {\n" +
		"	ID string `json:\"id\"`\n" +
		"	Note string `json:\"note,omitempty\"`\n" +
		"	Items []Item `json:\"items\"`\n" +
		"	Parent *Order `json:\"parent\"`\n" +
		"	Hidden string `json:\"-\"`\n" +
		"	NoTag int\n" +
		"	private int\n" +
		"}\n" +
		"type Item struct{}\n"
	s := extract(t, src)
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	order := s.Types[0]
	if order.Doc != "日本語🍵のコメントがあっても位置がずれない" {
		t.Errorf("doc = %q", order.Doc)
	}
	if got := slice(src, order.Span); got != "Order" {
		t.Errorf("type span covers %q", got)
	}

	want := []Field{
		{Name: "id", Tagged: true, TS: "string"},
		{Name: "note", Tagged: true, Optional: true, TS: "string"},
		{Name: "items", Tagged: true, TS: "Item[] | null"},
		{Name: "parent", Tagged: true, TS: "Order | null"},
		{Name: "NoTag", TS: "number"},
	}
	if len(order.Fields) != len(want) {
		t.Fatalf("fields = %+v", order.Fields)
	}
	for i, w := range want {
		f := order.Fields[i]
		if got := slice(src, f.Span); got != w.Name {
			t.Errorf("field %d span covers %q, want %q", i, got, w.Name)
		}
		w.Span = f.Span
		if f != w {
			t.Errorf("field %d = %+v, want %+v", i, f, w)
		}
	}
}

func TestConstsBecomeAUnion(t *testing.T) {
	s := extract(t, `package p
type Size string
const (
	SizeS Size = "S"
	SizeM Size = "M"
)
const Limit = 0x10
const hidden = 1
`)
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	if got := s.Types[0].Alias; got != `"S" | "M"` {
		t.Errorf("alias = %s", got)
	}
	if len(s.Consts) != 3 || s.Consts[2].Value != "16" {
		t.Errorf("consts = %+v", s.Consts)
	}
}

func TestFuncs(t *testing.T) {
	s := extract(t, `package p
type T struct{}
func A(t T, _ []string, m map[string]int) (*T, error) { return nil, nil }
func B() error { return nil }
func C() {}
func (t T) Method() {}
func d() {}
`)
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	if len(s.Funcs) != 3 {
		t.Fatalf("funcs = %+v", s.Funcs)
	}
	a := s.Funcs[0]
	if a.Result != "T | null" || !a.Throws {
		t.Errorf("A = %+v", a)
	}
	if a.Params[1] != (Param{"arg1", "string[] | null"}) {
		t.Errorf("A param 1 = %+v", a.Params[1])
	}
	if a.Params[2].TS != "Partial<Record<string, number>> | null" {
		t.Errorf("A param 2 = %+v", a.Params[2])
	}
	if b := s.Funcs[1]; b.Result != "void" || !b.Throws {
		t.Errorf("B = %+v", b)
	}
	if c := s.Funcs[2]; c.Result != "void" || c.Throws {
		t.Errorf("C = %+v", c)
	}
}

func TestDiagnostics(t *testing.T) {
	tests := []struct {
		name, src, covers, message string
	}{
		{"channel", "package p\nfunc F(c chan int) {}\n", "chan int", "cannot be sent"},
		{"int64", "package p\ntype T struct { N int64 }\n", "int64", "JavaScript number"},
		{"foreign type", "package p\nfunc F(o Other) {}\n", "Other", "not declared in this file"},
		{"unexported", "package p\ntype t struct{}\nfunc F(v t) {}\n", "t", "unexported"},
		{"variadic", "package p\nfunc F(a ...int) {}\n", "...int", "Variadic"},
		{"two results", "package p\nfunc F() (int, string) { return 0, \"\" }\n", "F", "one value"},
		{"embedded", "package p\ntype A struct{}\ntype B struct { A }\n", "A", "Embedded"},
		{"computed const", "package p\nconst N = 1 + 1\n", "N", "literal"},
		{"package main", "package main\n", "main", "package main"},
		{"syntax", "package p\nfunc F( {\n", "{", "expected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := extract(t, tt.src)
			if len(s.Diagnostics) == 0 {
				t.Fatal("no diagnostics")
			}
			d := s.Diagnostics[0]
			if !strings.Contains(d.Message, tt.message) {
				t.Errorf("message = %q, want %q", d.Message, tt.message)
			}
			got := slice(tt.src, Span{d.Start, d.Start + d.Length})
			if tt.name != "syntax" && got != tt.covers {
				t.Errorf("diagnostic covers %q, want %q", got, tt.covers)
			}
		})
	}
}
