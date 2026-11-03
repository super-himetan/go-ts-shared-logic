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

// Go code can return an unexported const, so the union has to include it.
func TestUnexportedConstsStayInTheUnion(t *testing.T) {
	s := extract(t, `package p
type Color string
const (
	Red Color = "red"
	internalBlue Color = "blue"
)
`)
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	if got := s.Types[0].Alias; got != `"red" | "blue"` {
		t.Errorf("alias = %s", got)
	}
	if len(s.Consts) != 1 || s.Consts[0].Name != "Red" {
		t.Errorf("consts = %+v", s.Consts)
	}
}

// A const whose value the parser cannot read could be anything, so no union can be claimed.
func TestComputedUnexportedConstOpensTheUnion(t *testing.T) {
	s := extract(t, `package p
type Color string
const (
	Red Color = "red"
	mixed Color = "a" + "b"
)
type Kind int
const (
	first Kind = iota
	second
)
`)
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	if got := s.Types[0].Alias; got != "string" {
		t.Errorf("Color = %s", got)
	}
	if got := s.Types[1].Alias; got != "number" {
		t.Errorf("Kind = %s", got)
	}
}

func TestReservedParameterNames(t *testing.T) {
	s := extract(t, `package p
func F(class string, function int, await bool, ok bool) {}
`)
	var names []string
	for _, p := range s.Funcs[0].Params {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, " "); got != "class_ function_ await_ ok" {
		t.Errorf("params = %s", got)
	}
}

func TestTagWrittenWithEscapes(t *testing.T) {
	src := "package p\ntype T struct { A int \"json:\\\"wire\\\"\" }\n"
	f := extract(t, src).Types[0].Fields[0]
	if f.Name != "wire" || f.Tagged || slice(src, f.Span) != "A" {
		t.Errorf("field = %+v", f)
	}
}

func TestOtherTagsAreNotMistakenForJSON(t *testing.T) {
	src := "package p\ntype T struct { A int `myjson:\"other\" json:\"wire\"` }\n"
	f := extract(t, src).Types[0].Fields[0]
	if f.Name != "wire" || slice(src, f.Span) != "wire" {
		t.Errorf("field = %+v", f)
	}
}

func TestBoolAndNegativeConsts(t *testing.T) {
	s := extract(t, `package p
const Debug = true
const Min = -1
const Low = -0.5
type Flag bool
const On Flag = true
const Bad = -"x"
`)
	if len(s.Consts) != 4 || s.Consts[0].Value != "true" || s.Consts[1].Value != "-1" || s.Consts[2].Value != "-0.5" {
		t.Errorf("consts = %+v", s.Consts)
	}
	if s.Types[0].Alias != "boolean" {
		t.Errorf("Flag = %s", s.Types[0].Alias)
	}
	if len(s.Diagnostics) != 1 {
		t.Errorf("diagnostics = %+v", s.Diagnostics)
	}
}

func TestInlineStructAndMapKeys(t *testing.T) {
	s := extract(t, "package p\ntype K string\ntype T struct {\n"+
		"	Items []struct { A int `json:\"a\"`; B string `json:\"b-c,omitempty\"` } `json:\"items\"`\n"+
		"	ByID map[int]string `json:\"byId\"`\n"+
		"	ByKey map[K]bool `json:\"byKey\"`\n"+
		"}\n")
	if len(s.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", s.Diagnostics)
	}
	fields := s.Types[1].Fields
	if got := fields[0].TS; got != `{ a: number; "b-c"?: string }[] | null` {
		t.Errorf("Items = %s", got)
	}
	if got := fields[1].TS; got != "{ [__K in number]?: string } | null" {
		t.Errorf("ByID = %s", got)
	}
	if got := fields[2].TS; got != "{ [__K in K]?: boolean } | null" {
		t.Errorf("ByKey = %s", got)
	}
}

func TestTimeIsResolvedThroughImports(t *testing.T) {
	s := extract(t, `package p
import clock "time"
type T struct { At clock.Time `+"`json:\"at\"`"+` }
`)
	if len(s.Diagnostics) != 0 || s.Types[0].Fields[0].TS != "string" {
		t.Errorf("schema = %+v", s)
	}
	s = extract(t, `package p
import time "example.com/notstd"
type T struct { At time.Time }
`)
	if len(s.Diagnostics) != 1 {
		t.Errorf("diagnostics = %+v", s.Diagnostics)
	}
}

func TestTrailingComments(t *testing.T) {
	s := extract(t, "package p\ntype T struct {\n\tA int // 合計\n\tB int // 小計\n}\ntype K string // 種類\nconst X = 1 // 上限\n")
	if got := s.Types[0].Fields[1].Doc; got != "小計" {
		t.Errorf("field doc = %q", got)
	}
	if got := s.Types[1].Doc; got != "種類" {
		t.Errorf("type doc = %q", got)
	}
	if got := s.Consts[0].Doc; got != "上限" {
		t.Errorf("const doc = %q", got)
	}
}

func TestNameCollisions(t *testing.T) {
	s := extract(t, "package p\nfunc F(class string, class_ string) {}\ntype T struct { A int `json:\"x\"`; B int `json:\"x\"` }\n")
	if p := s.Funcs[0].Params; p[0].Name != "class_" || p[1].Name != "class__" {
		t.Errorf("params = %+v", p)
	}
	if len(s.Types[0].Fields) != 1 || len(s.Diagnostics) != 1 || !strings.Contains(s.Diagnostics[0].Message, "also named 'x'") {
		t.Errorf("schema = %+v", s)
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
	if a.Params[2].TS != "{ [__K in string]?: number } | null" {
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
		{"channel", "package p\nfunc F(c chan int) {}\n", "chan int", "no JSON form"},
		{"int64", "package p\ntype T struct { N int64 }\n", "int64", "JavaScript number"},
		{"foreign type", "package p\nfunc F(o Other) {}\n", "Other", "not declared in this file"},
		{"unexported", "package p\ntype t struct{}\nfunc F(v t) {}\n", "t", "unexported"},
		{"variadic", "package p\nfunc F(a ...int) {}\n", "...int", "Variadic"},
		{"two results", "package p\nfunc F() (int, string) { return 0, \"\" }\n", "F", "one value"},
		{"embedded", "package p\ntype A struct{}\ntype B struct { A }\n", "A", "Embedded"},
		{"computed const", "package p\nconst N = 1 + 1\n", "N", "literal"},
		{"package main", "package main\n", "main", "package main"},
		{"MarshalJSON", "package p\ntype T struct{}\nfunc (t T) MarshalJSON() ([]byte, error) { return nil, nil }\n", "MarshalJSON", "customizes its encoding"},
		{"pointer UnmarshalText", "package p\ntype T struct{}\nfunc (t *T) UnmarshalText(b []byte) error { return nil }\n", "UnmarshalText", "customizes its encoding"},
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
