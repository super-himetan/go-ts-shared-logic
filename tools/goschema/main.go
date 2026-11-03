// Command goschema reads one Go file from stdin and prints what it exports as
// JSON, with TypeScript types and UTF-16 source spans. It uses go/parser only,
// so it also works on an unsaved file in an editor.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	codeSyntax = 9200 + iota
	codeType
	codeSignature
	codeDeclaration
)

type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type Diagnostic struct {
	Code    int    `json:"code"`
	Message string `json:"messageText"`
	Start   int    `json:"start"`
	Length  int    `json:"length"`
}

type Field struct {
	Name string `json:"name"`
	Span Span   `json:"span"`
	// Tagged fields can be renamed from TypeScript: only the wire name changes.
	Tagged   bool   `json:"tagged"`
	Optional bool   `json:"optional"`
	TS       string `json:"ts"`
	Doc      string `json:"doc,omitempty"`
}

type Type struct {
	Name   string  `json:"name"`
	Span   Span    `json:"span"`
	Doc    string  `json:"doc,omitempty"`
	Fields []Field `json:"fields,omitempty"`
	// Alias is the right-hand side when the type is not a struct.
	Alias string `json:"alias,omitempty"`
}

type Param struct {
	Name string `json:"name"`
	TS   string `json:"ts"`
}

type Func struct {
	Name   string  `json:"name"`
	Span   Span    `json:"span"`
	Doc    string  `json:"doc,omitempty"`
	Params []Param `json:"params"`
	Result string  `json:"result"`
	Throws bool    `json:"throws"`
}

type Const struct {
	Name string `json:"name"`
	Span Span   `json:"span"`
	Doc  string `json:"doc,omitempty"`
	// Value is a JSON literal, which is also a TypeScript literal.
	Value string `json:"value"`
}

type Schema struct {
	Package     string       `json:"package"`
	Types       []Type       `json:"types"`
	Funcs       []Func       `json:"funcs"`
	Consts      []Const      `json:"consts"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type extractor struct {
	schema   Schema
	fset     *token.FileSet
	utf16At  []int
	declared map[string]bool
	// literals holds the const values of each named type, which become its union.
	literals map[string][]string
	// openTypes have a const whose value cannot be read here, so a union would be a lie.
	openTypes map[string]bool
	// imports maps the name a package has in this file to its import path.
	imports map[string]string
}

// encodingMethods replace what encoding/json writes for a type, which the
// parser cannot see through.
var encodingMethods = map[string]bool{
	"MarshalJSON":   true,
	"UnmarshalJSON": true,
	"MarshalText":   true,
	"UnmarshalText": true,
}

// reservedWords are valid Go parameter names that TypeScript does not allow.
var reservedWords = map[string]bool{
	"arguments": true, "await": true, "catch": true, "class": true, "debugger": true,
	"delete": true, "do": true, "enum": true, "eval": true, "export": true,
	"extends": true, "false": true, "finally": true, "function": true, "implements": true,
	"in": true, "instanceof": true, "let": true, "new": true, "null": true,
	"private": true, "protected": true, "public": true, "static": true, "super": true,
	"this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"void": true, "while": true, "with": true, "yield": true,
}

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(Extract(src)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Extract never fails: every problem becomes a diagnostic on the source.
func Extract(src []byte) Schema {
	e := &extractor{
		schema: Schema{
			Types:       []Type{},
			Funcs:       []Func{},
			Consts:      []Const{},
			Diagnostics: []Diagnostic{},
		},
		fset:      token.NewFileSet(),
		utf16At:   utf16Offsets(src),
		declared:  map[string]bool{},
		literals:  map[string][]string{},
		openTypes: map[string]bool{},
		imports:   map[string]string{},
	}

	file, err := parser.ParseFile(e.fset, "input.go", src, parser.ParseComments|parser.AllErrors)
	var syntaxErrors scanner.ErrorList
	if errors.As(err, &syntaxErrors) {
		for _, syntaxError := range syntaxErrors {
			start := e.utf16At[min(syntaxError.Pos.Offset, len(src))]
			e.report(codeSyntax, Span{start, start + 1}, syntaxError.Msg)
		}
	}
	if file == nil {
		return e.schema
	}

	e.schema.Package = file.Name.Name
	if file.Name.Name == "main" {
		e.report(codeDeclaration, e.identSpan(file.Name), "package main cannot be imported, so its functions cannot be exported.")
	}

	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		e.imports[name] = importPath
	}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				e.declared[spec.(*ast.TypeSpec).Name.Name] = true
			}
		}
	}
	// Consts first, so a type declared above its consts still gets its union.
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.CONST {
			e.addConsts(gen)
		}
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					e.addType(d, spec.(*ast.TypeSpec))
				}
			}
		case *ast.FuncDecl:
			e.addFunc(d)
		}
	}
	return e.schema
}

func (e *extractor) addConsts(gen *ast.GenDecl) {
	for _, spec := range gen.Specs {
		value := spec.(*ast.ValueSpec)
		typeName := ""
		if ident, ok := value.Type.(*ast.Ident); ok && e.declared[ident.Name] {
			typeName = ident.Name
		}
		for i, name := range value.Names {
			var literal string
			if i < len(value.Values) {
				literal = jsonLiteral(value.Values[i])
			}
			if literal == "" {
				if name.IsExported() {
					e.report(codeDeclaration, e.identSpan(name), "Only consts written as a literal can be exported to TypeScript.")
				}
				if typeName != "" {
					e.openTypes[typeName] = true
				}
				continue
			}
			// Unexported consts are not exported, but Go code can still return them.
			if typeName != "" {
				e.literals[typeName] = append(e.literals[typeName], literal)
				// The other value of a bool is always there, as its zero value.
				if literal == "true" || literal == "false" {
					e.openTypes[typeName] = true
				}
			}
			if !name.IsExported() {
				continue
			}
			e.schema.Consts = append(e.schema.Consts, Const{
				Name:  name.Name,
				Span:  e.identSpan(name),
				Doc:   specDoc(value.Doc, value.Comment, gen, len(gen.Specs)),
				Value: literal,
			})
		}
	}
}

func (e *extractor) addType(gen *ast.GenDecl, spec *ast.TypeSpec) {
	if !spec.Name.IsExported() {
		return
	}
	t := Type{Name: spec.Name.Name, Span: e.identSpan(spec.Name), Doc: specDoc(spec.Doc, spec.Comment, gen, len(gen.Specs))}
	if spec.TypeParams != nil {
		e.report(codeDeclaration, t.Span, "Generic types are not supported.")
		t.Alias = "unknown"
		e.schema.Types = append(e.schema.Types, t)
		return
	}
	if structType, ok := spec.Type.(*ast.StructType); ok {
		t.Fields = e.fields(structType)
		if t.Fields == nil {
			t.Fields = []Field{}
		}
	} else if literals := e.literals[spec.Name.Name]; len(literals) > 0 && !e.openTypes[spec.Name.Name] {
		t.Alias = strings.Join(literals, " | ")
	} else {
		t.Alias = e.tsType(spec.Type)
	}
	e.schema.Types = append(e.schema.Types, t)
}

func (e *extractor) fields(structType *ast.StructType) []Field {
	var fields []Field
	seen := map[string]bool{}
	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			e.report(codeType, e.span(field.Type), "Embedded fields are not supported.")
			continue
		}
		ts := e.tsType(field.Type)
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			f := Field{Name: name.Name, Span: e.identSpan(name), TS: ts, Doc: docOf(field.Doc, field.Comment, 1)}
			if field.Tag != nil {
				if skip := e.applyTag(&f, field.Tag); skip {
					continue
				}
			}
			// encoding/json drops both fields, so neither reaches the other side.
			if seen[f.Name] {
				e.report(codeType, f.Span, fmt.Sprintf("Another field in this struct is also named '%s' in JSON.", f.Name))
				continue
			}
			seen[f.Name] = true
			fields = append(fields, f)
		}
	}
	return fields
}

// applyTag applies a `json:"..."` tag and reports whether the field is left out of JSON.
func (e *extractor) applyTag(f *Field, tag *ast.BasicLit) bool {
	unquoted, err := strconv.Unquote(tag.Value)
	if err != nil {
		return false
	}
	value, ok := reflect.StructTag(unquoted).Lookup("json")
	if !ok {
		return false
	}
	if value == "-" {
		return true
	}
	name, options, _ := strings.Cut(value, ",")
	for option := range strings.SplitSeq(options, ",") {
		switch option {
		case "omitempty", "omitzero":
			f.Optional = true
		case "string":
			e.report(codeType, e.span(tag), "The ',string' JSON option is not supported.")
		}
	}
	if name == "" {
		return false
	}
	f.Name = name
	// Spans must land on the name inside the tag, so "go to definition" from TypeScript finds it.
	// A tag written with escapes has no such text, so it keeps the span of the field.
	if i := strings.Index(tag.Value, `json:"`+name); i >= 0 {
		start := e.fset.Position(tag.Pos()).Offset + i + len(`json:"`)
		f.Span = Span{e.utf16At[start], e.utf16At[start+len(name)]}
		f.Tagged = true
	}
	return false
}

func (e *extractor) addFunc(fn *ast.FuncDecl) {
	if fn.Recv != nil {
		e.checkEncodingMethod(fn)
		return
	}
	if !fn.Name.IsExported() {
		return
	}
	span := e.identSpan(fn.Name)
	if fn.Type.TypeParams != nil {
		e.report(codeSignature, span, "Generic functions are not supported.")
		return
	}

	f := Func{Name: fn.Name.Name, Span: span, Doc: docOf(fn.Doc, nil, 0), Params: []Param{}, Result: "void"}
	used := map[string]bool{}
	addParam := func(name, ts string) {
		for used[name] {
			name += "_"
		}
		used[name] = true
		f.Params = append(f.Params, Param{Name: name, TS: ts})
	}
	for _, field := range fn.Type.Params.List {
		if _, variadic := field.Type.(*ast.Ellipsis); variadic {
			e.report(codeSignature, e.span(field.Type), "Variadic functions are not supported.")
			return
		}
		ts := e.tsType(field.Type)
		if len(field.Names) == 0 {
			addParam(fmt.Sprintf("arg%d", len(f.Params)), ts)
		}
		for _, name := range field.Names {
			paramName := name.Name
			if paramName == "_" {
				paramName = fmt.Sprintf("arg%d", len(f.Params))
			} else if reservedWords[paramName] {
				paramName += "_"
			}
			addParam(paramName, ts)
		}
	}

	var results []*ast.Field
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			for range max(len(field.Names), 1) {
				results = append(results, field)
			}
		}
	}
	if n := len(results); n > 0 && isError(results[n-1].Type) {
		f.Throws = true
		results = results[:n-1]
	}
	switch len(results) {
	case 0:
	case 1:
		f.Result = e.tsType(results[0].Type)
	default:
		e.report(codeSignature, span, "A function can return one value and an optional error. Return a struct instead.")
		return
	}
	e.schema.Funcs = append(e.schema.Funcs, f)
}

func (e *extractor) checkEncodingMethod(fn *ast.FuncDecl) {
	if !encodingMethods[fn.Name.Name] || len(fn.Recv.List) == 0 {
		return
	}
	receiver := fn.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if ident, ok := receiver.(*ast.Ident); ok && ident.IsExported() {
		e.report(codeType, e.identSpan(fn.Name), fmt.Sprintf("'%s' customizes its encoding with %s, so its TypeScript type cannot be derived.", ident.Name, fn.Name.Name))
	}
}

// tsType matches what encoding/json actually writes. A nil slice or map becomes
// null, so that stays in the type rather than being a surprise at runtime.
func (e *extractor) tsType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return "string"
		case "bool":
			return "boolean"
		case "int", "int8", "int16", "int32", "uint8", "uint16", "uint32", "byte", "rune", "float32", "float64":
			return "number"
		case "int64", "uint64", "uint", "uintptr":
			e.report(codeType, e.span(t), fmt.Sprintf("'%s' can exceed what a JavaScript number holds exactly. Use int, or a string.", t.Name))
			return "number"
		case "any":
			return "unknown"
		}
		if !e.declared[t.Name] {
			e.report(codeType, e.span(t), fmt.Sprintf("'%s' is not declared in this file, so it has no TypeScript type.", t.Name))
			return "unknown"
		}
		if !t.IsExported() {
			e.report(codeType, e.span(t), fmt.Sprintf("'%s' is unexported, so TypeScript cannot name it.", t.Name))
			return "unknown"
		}
		return t.Name
	case *ast.StarExpr:
		return e.tsType(t.X) + " | null"
	case *ast.ArrayType:
		if ident, ok := t.Elt.(*ast.Ident); ok && t.Len == nil && (ident.Name == "byte" || ident.Name == "uint8") {
			return "string | null" // encoding/json writes []byte as base64
		}
		element := e.tsType(t.Elt)
		if strings.Contains(element, " | ") {
			element = "(" + element + ")"
		}
		if t.Len != nil {
			return element + "[]"
		}
		return element + "[] | null"
	case *ast.MapType:
		key := e.tsType(t.Key)
		if ident, ok := t.Key.(*ast.Ident); !ok || ident.Name == "float32" || ident.Name == "float64" || key == "boolean" || key == "unknown" {
			e.report(codeType, e.span(t.Key), "Only string and integer keys can be written to JSON.")
		}
		// Not Partial<Record<...>>, which a Go type named Record would shadow.
		return "{ [__K in " + key + "]?: " + e.tsType(t.Value) + " } | null"
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && e.imports[pkg.Name] == "time" && t.Sel.Name == "Time" {
			return "string"
		}
	case *ast.StructType:
		var members []string
		for _, field := range e.fields(t) {
			name := field.Name
			if !token.IsIdentifier(name) {
				name = strconv.Quote(name)
			}
			if field.Optional {
				name += "?"
			}
			members = append(members, name+": "+field.TS)
		}
		return "{ " + strings.Join(members, "; ") + " }"
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return "unknown"
		}
	}
	e.report(codeType, e.span(expr), "This type has no JSON form that TypeScript can describe.")
	return "unknown"
}

func (e *extractor) report(code int, span Span, message string) {
	e.schema.Diagnostics = append(e.schema.Diagnostics, Diagnostic{
		Code:    code,
		Message: message,
		Start:   span.Start,
		Length:  max(span.End-span.Start, 1),
	})
}

func (e *extractor) span(node ast.Node) Span {
	return Span{
		e.utf16At[e.fset.Position(node.Pos()).Offset],
		e.utf16At[e.fset.Position(node.End()).Offset],
	}
}

func (e *extractor) identSpan(ident *ast.Ident) Span {
	return e.span(ident)
}

func isError(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}

// specDoc prefers a spec's own comments over the comment of its group.
func specDoc(doc, comment *ast.CommentGroup, gen *ast.GenDecl, specs int) string {
	if text := docOf(doc, comment, 1); text != "" {
		return text
	}
	return docOf(nil, gen.Doc, specs)
}

// docOf falls back to the group's comment only when the group has a single spec.
func docOf(doc, groupDoc *ast.CommentGroup, specs int) string {
	if doc == nil && specs == 1 {
		doc = groupDoc
	}
	return strings.TrimSpace(doc.Text())
}

func jsonLiteral(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return v.Name
		}
		return ""
	case *ast.UnaryExpr:
		if number, ok := v.X.(*ast.BasicLit); ok && v.Op == token.SUB && number.Kind != token.STRING {
			if literal := jsonLiteral(number); literal != "" {
				return "-" + literal
			}
		}
		return ""
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok {
		return ""
	}
	switch lit.Kind {
	case token.STRING:
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return ""
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	case token.INT:
		value, err := strconv.ParseInt(lit.Value, 0, 54)
		if err != nil {
			return ""
		}
		return strconv.FormatInt(value, 10)
	case token.FLOAT:
		value, err := strconv.ParseFloat(lit.Value, 64)
		if err != nil {
			return ""
		}
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
	return ""
}

// utf16Offsets maps a byte offset in src to an offset in the JavaScript string
// holding the same text. It matters as soon as a comment contains Japanese.
func utf16Offsets(src []byte) []int {
	offsets := make([]int, len(src)+1)
	units := 0
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		for j := range size {
			offsets[i+j] = units
		}
		if r >= 0x10000 {
			units += 2
		} else {
			units++
		}
		i += size
	}
	offsets[len(src)] = units
	return offsets
}
