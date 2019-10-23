package gen

import (
	"fmt"
	"go/types"
	"runtime"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// Target this byte size for initial slice allocation to reduce garbage collection.
const minSliceBytes = 64

func (g *Generator) getDecoderName(t types.Type) string {
	return g.functionName("decode", t)
}

var primitiveDecoders = map[types.BasicKind]string{
	types.String:  "in.String()",
	types.Bool:    "in.Bool()",
	types.Int:     "in.Int()",
	types.Int8:    "in.Int8()",
	types.Int16:   "in.Int16()",
	types.Int32:   "in.Int32()",
	types.Int64:   "in.Int64()",
	types.Uint:    "in.Uint()",
	types.Uint8:   "in.Uint8()",
	types.Uint16:  "in.Uint16()",
	types.Uint32:  "in.Uint32()",
	types.Uint64:  "in.Uint64()",
	types.Float32: "in.Float32()",
	types.Float64: "in.Float64()",
}

var primitiveStringDecoders = map[types.BasicKind]string{
	types.String:  "in.String()",
	types.Int:     "in.IntStr()",
	types.Int8:    "in.Int8Str()",
	types.Int16:   "in.Int16Str()",
	types.Int32:   "in.Int32Str()",
	types.Int64:   "in.Int64Str()",
	types.Uint:    "in.UintStr()",
	types.Uint8:   "in.Uint8Str()",
	types.Uint16:  "in.Uint16Str()",
	types.Uint32:  "in.Uint32Str()",
	types.Uint64:  "in.Uint64Str()",
	types.Uintptr: "in.UintptrStr()",
	types.Float32: "in.Float32Str()",
	types.Float64: "in.Float64Str()",
}

var customDecoders = map[string]string{
	"json.Number": "in.JsonNumber()",
}

// implements returns false if t doesn't implement i
//
// The standard types.Implements panics on false, making it inconvenient for
// simple checking.
func implements(t types.Type, i *types.Interface) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	ok = types.Implements(t, i)

	return
}

var easyjsonUnmarshaler, jsonUnmarshaler, encodingTextUnmarshaler *types.Interface

func init() {
	pkgs, err := packages.Load(
		&packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo},
		"encoding", "encoding/json", "github.com/mailru/easyjson",
	)
	if err != nil {
		panic(err.Error())
	}

	if len(pkgs) != 3 {
		panic("should have found exactly three packages (for encoding, encoding/json, and github.com/mailru/easyjson)")
	}

	// pkg[0]: encoding
	for _, o := range pkgs[0].TypesInfo.Defs {
		if o == nil {
			continue
		}
		switch o.Name() {
			case "TextUnmarshaler":
				encodingTextUnmarshaler = o.Type().Underlying().(*types.Interface)
			case "TextMarshaler":
				encodingTextMarshaler = o.Type().Underlying().(*types.Interface)
		}
	}

	// pkg[1]: encoding/json
	for _, o := range pkgs[1].TypesInfo.Defs {
		if o == nil {
			continue
		}
		switch o.Name() {
			case "Unmarshaler":
				jsonUnmarshaler = o.Type().Underlying().(*types.Interface)
			case "Marshaler":
				jsonMarshaler = o.Type().Underlying().(*types.Interface)
		}
	}

	// pkg[2]: github.com/mailru/easyjson
	for _, o := range pkgs[2].TypesInfo.Defs {
		if o == nil {
			continue
		}
		switch o.Name() {
			case "Unmarshaler":
				easyjsonUnmarshaler = o.Type().Underlying().(*types.Interface)
			case "Marshaler":
				easyjsonMarshaler = o.Type().Underlying().(*types.Interface)
			case "Optional":
				easyjsonOptional = o.Type().Underlying().(*types.Interface)
		}
	}

	switch {
	case encodingTextUnmarshaler == nil:
			panic("encodingTextUnmarshaler not set")
	case encodingTextMarshaler == nil:
			panic("encodingTextMarshaler not set")
	case jsonUnmarshaler == nil:
			panic("jsonUnmarshaler not set")
	case jsonMarshaler == nil:
			panic("jsonMarshaler not set")
	case easyjsonUnmarshaler == nil:
			panic("easyjsonUnmarshaler not set")
	case easyjsonUnmarshaler == nil:
			panic("easyjsonUnmarshaler not set")
	case easyjsonOptional == nil:
			panic("easyjsonOptional not set")
	}
}

// genTypeDecoder generates decoding code for the type t, but uses unmarshaler interface if implemented by t.
func (g *Generator) genTypeDecoder(t types.Type, out string, tags fieldTags, indent int) error {
	ws := strings.Repeat("  ", indent)

	p := types.NewPointer(t)

	if implements(p, easyjsonUnmarshaler) {
		fmt.Fprintln(g.out, ws+"("+out+").UnmarshalEasyJSON(in)")
		return nil
	}

	if implements(p, jsonUnmarshaler) {
		fmt.Fprintln(g.out, ws+"if data := in.Raw(); in.Ok() {")
		fmt.Fprintln(g.out, ws+"  in.AddError( ("+out+").UnmarshalJSON(data) )")
		fmt.Fprintln(g.out, ws+"}")
		return nil
	}

	if implements(p, encodingTextUnmarshaler) {
		fmt.Fprintln(g.out, ws+"if data := in.UnsafeBytes(); in.Ok() {")
		fmt.Fprintln(g.out, ws+"  in.AddError( ("+out+").UnmarshalText(data) )")
		fmt.Fprintln(g.out, ws+"}")
		return nil
	}

	err := g.genTypeDecoderNoCheck(t, out, tags, indent)
	return err
}

// returns true of the type t implements one of the custom unmarshaler interfaces
func hasCustomUnmarshaler(t types.Type) bool {
	t = types.NewPointer(t)
	return implements(t, easyjsonUnmarshaler) ||
		implements(t, jsonUnmarshaler) ||
		implements(t, encodingTextUnmarshaler)
}

// genTypeDecoderNoCheck generates decoding code for the type t.
func (g *Generator) genTypeDecoderNoCheck(t types.Type, out string, tags fieldTags, indent int) error {
	ws := strings.Repeat("  ", indent)
	// Check whether type is primitive, needs to be done after interface check.
	if dec := customDecoders[t.String()]; dec != "" {
		fmt.Fprintln(g.out, ws+out+" = "+dec)
		return nil
	} else if b, ok := t.(*types.Basic); ok {
		if dec := primitiveStringDecoders[b.Kind()]; dec != "" && tags.asString {
			fmt.Fprintln(g.out, ws+out+" = "+g.getType(t)+"("+dec+")")
			return nil
		} else if dec := primitiveDecoders[b.Kind()]; dec != "" {
			fmt.Fprintln(g.out, ws+out+" = "+g.getType(t)+"("+dec+")")
			return nil
		}
	}

	switch v := t.Underlying().(type) {
	case *types.Slice:
		tmpVar := g.uniqueVarName()
		elem := v.Elem()

		if b, ok := elem.(*types.Basic); ok && b.Kind() == types.Uint8 {
			fmt.Fprintln(g.out, ws+"if in.IsNull() {")
			fmt.Fprintln(g.out, ws+"  in.Skip()")
			fmt.Fprintln(g.out, ws+"  "+out+" = nil")
			fmt.Fprintln(g.out, ws+"} else {")
			fmt.Fprintln(g.out, ws+"  "+out+" = in.Bytes()")
			fmt.Fprintln(g.out, ws+"}")

		} else {
			capacity := minSliceBytes / types.SizesFor(runtime.Compiler, runtime.GOARCH).Sizeof(elem)
			if capacity == 0 {
				capacity = 1
			}

			fmt.Fprintln(g.out, ws+"if in.IsNull() {")
			fmt.Fprintln(g.out, ws+"  in.Skip()")
			fmt.Fprintln(g.out, ws+"  "+out+" = nil")
			fmt.Fprintln(g.out, ws+"} else {")
			fmt.Fprintln(g.out, ws+"  in.Delim('[')")
			fmt.Fprintln(g.out, ws+"  if "+out+" == nil {")
			fmt.Fprintln(g.out, ws+"    if !in.IsDelim(']') {")
			fmt.Fprintln(g.out, ws+"      "+out+" = make("+g.getType(t)+", 0, "+fmt.Sprint(capacity)+")")
			fmt.Fprintln(g.out, ws+"    } else {")
			fmt.Fprintln(g.out, ws+"      "+out+" = "+g.getType(t)+"{}")
			fmt.Fprintln(g.out, ws+"    }")
			fmt.Fprintln(g.out, ws+"  } else { ")
			fmt.Fprintln(g.out, ws+"    "+out+" = ("+out+")[:0]")
			fmt.Fprintln(g.out, ws+"  }")
			fmt.Fprintln(g.out, ws+"  for !in.IsDelim(']') {")
			fmt.Fprintln(g.out, ws+"    var "+tmpVar+" "+g.getType(elem))

			if err := g.genTypeDecoder(elem, tmpVar, tags, indent+2); err != nil {
				return err
			}

			fmt.Fprintln(g.out, ws+"    "+out+" = append("+out+", "+tmpVar+")")
			fmt.Fprintln(g.out, ws+"    in.WantComma()")
			fmt.Fprintln(g.out, ws+"  }")
			fmt.Fprintln(g.out, ws+"  in.Delim(']')")
			fmt.Fprintln(g.out, ws+"}")
		}

	case *types.Array:
		iterVar := g.uniqueVarName()
		elem := v.Elem()

		if b, ok := elem.(*types.Basic); ok && b.Kind() == types.Uint8 {
			fmt.Fprintln(g.out, ws+"if in.IsNull() {")
			fmt.Fprintln(g.out, ws+"  in.Skip()")
			fmt.Fprintln(g.out, ws+"} else {")
			fmt.Fprintln(g.out, ws+"  copy("+out+"[:], in.Bytes())")
			fmt.Fprintln(g.out, ws+"}")

		} else {

			length := v.Len()

			fmt.Fprintln(g.out, ws+"if in.IsNull() {")
			fmt.Fprintln(g.out, ws+"  in.Skip()")
			fmt.Fprintln(g.out, ws+"} else {")
			fmt.Fprintln(g.out, ws+"  in.Delim('[')")
			fmt.Fprintln(g.out, ws+"  "+iterVar+" := 0")
			fmt.Fprintln(g.out, ws+"  for !in.IsDelim(']') {")
			fmt.Fprintln(g.out, ws+"    if "+iterVar+" < "+fmt.Sprint(length)+" {")

			if err := g.genTypeDecoder(elem, "("+out+")["+iterVar+"]", tags, indent+3); err != nil {
				return err
			}

			fmt.Fprintln(g.out, ws+"      "+iterVar+"++")
			fmt.Fprintln(g.out, ws+"    } else {")
			fmt.Fprintln(g.out, ws+"      in.SkipRecursive()")
			fmt.Fprintln(g.out, ws+"    }")
			fmt.Fprintln(g.out, ws+"    in.WantComma()")
			fmt.Fprintln(g.out, ws+"  }")
			fmt.Fprintln(g.out, ws+"  in.Delim(']')")
			fmt.Fprintln(g.out, ws+"}")
		}

	case *types.Struct:
		dec := g.getDecoderName(t)
		g.addType(t)

		if len(out) > 0 && out[0] == '*' {
			// NOTE: In order to remove an extra reference to a pointer
			fmt.Fprintln(g.out, ws+dec+"(in, "+out[1:]+")")
		} else {
			fmt.Fprintln(g.out, ws+dec+"(in, &"+out+")")
		}

	case *types.Pointer:
		fmt.Fprintln(g.out, ws+"if in.IsNull() {")
		fmt.Fprintln(g.out, ws+"  in.Skip()")
		fmt.Fprintln(g.out, ws+"  "+out+" = nil")
		fmt.Fprintln(g.out, ws+"} else {")
		fmt.Fprintln(g.out, ws+"  if "+out+" == nil {")
		fmt.Fprintln(g.out, ws+"    "+out+" = new("+g.getType(v.Elem())+")")
		fmt.Fprintln(g.out, ws+"  }")

		if err := g.genTypeDecoder(v.Elem(), "*"+out, tags, indent+1); err != nil {
			return err
		}

		fmt.Fprintln(g.out, ws+"}")

	case *types.Map:
		key := v.Key()
		b, ok := key.(*types.Basic)
		if !ok {
			return fmt.Errorf("map type %v not supported: only string and integer keys and types implementing json.Unmarshaler are allowed", key)
		}
		keyDec, ok := primitiveStringDecoders[b.Kind()]
		if !ok && !hasCustomUnmarshaler(key) {
			return fmt.Errorf("map type %v not supported: only string and integer keys and types implementing json.Unmarshaler are allowed", key)
		} // else assume the caller knows what they are doing and that the custom unmarshaler performs the translation from string or integer keys to the key type
		elem := v.Elem()
		tmpVar := g.uniqueVarName()

		fmt.Fprintln(g.out, ws+"if in.IsNull() {")
		fmt.Fprintln(g.out, ws+"  in.Skip()")
		fmt.Fprintln(g.out, ws+"} else {")
		fmt.Fprintln(g.out, ws+"  in.Delim('{')")
		fmt.Fprintln(g.out, ws+"  if !in.IsDelim('}') {")
		fmt.Fprintln(g.out, ws+"  "+out+" = make("+g.getType(t)+")")
		fmt.Fprintln(g.out, ws+"  } else {")
		fmt.Fprintln(g.out, ws+"  "+out+" = nil")
		fmt.Fprintln(g.out, ws+"  }")

		fmt.Fprintln(g.out, ws+"  for !in.IsDelim('}') {")
		// NOTE: extra check for TextUnmarshaler. It overrides default methods.
		if implements(types.NewPointer(key), encodingTextUnmarshaler) {
			fmt.Fprintln(g.out, ws+"    var key "+g.getType(key))
			fmt.Fprintln(g.out, ws+"if data := in.UnsafeBytes(); in.Ok() {")
			fmt.Fprintln(g.out, ws+"  in.AddError(key.UnmarshalText(data) )")
			fmt.Fprintln(g.out, ws+"}")
		} else if keyDec != "" {
			fmt.Fprintln(g.out, ws+"    key := "+g.getType(key)+"("+keyDec+")")
		} else {
			fmt.Fprintln(g.out, ws+"    var key "+g.getType(key))
			if err := g.genTypeDecoder(key, "key", tags, indent+2); err != nil {
				return err
			}
		}

		fmt.Fprintln(g.out, ws+"    in.WantColon()")
		fmt.Fprintln(g.out, ws+"    var "+tmpVar+" "+g.getType(elem))

		if err := g.genTypeDecoder(elem, tmpVar, tags, indent+2); err != nil {
			return err
		}

		fmt.Fprintln(g.out, ws+"    ("+out+")[key] = "+tmpVar)
		fmt.Fprintln(g.out, ws+"    in.WantComma()")
		fmt.Fprintln(g.out, ws+"  }")
		fmt.Fprintln(g.out, ws+"  in.Delim('}')")
		fmt.Fprintln(g.out, ws+"}")

	case *types.Interface:
		if v.NumMethods() != 0 {
			return fmt.Errorf("interface type %v not supported: only interface{} is allowed", t)
		}
		fmt.Fprintln(g.out, ws+"if m, ok := "+out+".(easyjson.Unmarshaler); ok {")
		fmt.Fprintln(g.out, ws+"m.UnmarshalEasyJSON(in)")
		fmt.Fprintln(g.out, ws+"} else if m, ok := "+out+".(json.Unmarshaler); ok {")
		fmt.Fprintln(g.out, ws+"_ = m.UnmarshalJSON(in.Raw())")
		fmt.Fprintln(g.out, ws+"} else {")
		fmt.Fprintln(g.out, ws+"  "+out+" = in.Interface()")
		fmt.Fprintln(g.out, ws+"}")
	default:
		return fmt.Errorf("don't know how to decode %v", t)
	}
	return nil

}

func (g *Generator) genStructFieldDecoder(t *types.Struct, i int) error {
	jsonName := g.fieldNamer.GetJSONFieldName(t, i)
	tags := parseFieldTags(t.Tag(i))
	fieldVar := t.Field(i)

	if tags.omit {
		return nil
	}

	fmt.Fprintf(g.out, "    case %q:\n", jsonName)
	if err := g.genTypeDecoder(fieldVar.Type(), "out."+fieldVar.Name(), tags, 3); err != nil {
		return err
	}

	if tags.required {
		fmt.Fprintf(g.out, "%sSet = true\n", fieldVar.Name())
	}

	return nil
}

func (g *Generator) genRequiredFieldSet(t *types.Struct, i int) {
	tags := parseFieldTags(t.Tag(i))

	if !tags.required {
		return
	}

	fmt.Fprintf(g.out, "var %sSet bool\n", t.Field(i).Name())
}

func (g *Generator) genRequiredFieldCheck(t *types.Struct, i int) {
	jsonName := g.fieldNamer.GetJSONFieldName(t, i)
	tags := parseFieldTags(t.Tag(i))

	if !tags.required {
		return
	}

	g.imports["fmt"] = "fmt"

	fmt.Fprintf(g.out, "if !%sSet {\n", t.Field(i).Name())
	fmt.Fprintf(g.out, "    in.AddError(fmt.Errorf(\"key '%s' is required\"))\n", jsonName)
	fmt.Fprintf(g.out, "}\n")
}

func mergeStructFields(t1, t2 *types.Struct, f1, f2 []int) (fields []int) {
	used := map[string]bool{}
	for _, i := range f2 {
		used[t2.Field(i).Name()] = true
		fields = append(fields, i)
	}

	for _, i := range f1 {
		if !used[t1.Field(i).Name()] {
			fields = append(fields, i)
		}
	}
	return
}

func getStructFields(t *types.Struct) ([]int, error) {
	var efields []int
	for i := 0; i < t.NumFields(); i++ {
		f := t.Field(i)
		tags := parseFieldTags(t.Tag(i))
		if !f.Anonymous() || tags.name != "" {
			continue
		}

		t1 := f.Type()
		if p, ok := t1.(*types.Pointer); ok {
			t1 = p.Elem()
		}

		if s, ok := t1.(*types.Struct); ok {
			fs, err := getStructFields(s)
			if err != nil {
				return nil, fmt.Errorf("error processing embedded field: %v", err)
			}
			efields = mergeStructFields(t, s, efields, fs)
		}
	}

	var fields []int
	for i := 0; i < t.NumFields(); i++ {
		f := t.Field(i)
		tags := parseFieldTags(t.Tag(i))
		if f.Anonymous() && tags.name == "" {
			continue
		}

		c := []rune(f.Name())[0]
		if unicode.IsUpper(c) {
			fields = append(fields, i)
		}
	}
	return mergeStructFields(t, t, efields, fields), nil
}

func (g *Generator) genDecoder(t types.Type) error {
	switch t.(type) {
	case *types.Slice, *types.Array, *types.Map:
		return g.genSliceArrayDecoder(t)
	default:
		return g.genStructDecoder(t)
	}
}

func (g *Generator) genSliceArrayDecoder(t types.Type) error {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array, *types.Map:
	default:
		return fmt.Errorf("cannot generate decoder for %v, not a slice/array/map type", t)
	}

	fname := g.getDecoderName(t)
	typ := g.getType(t)

	fmt.Fprintln(g.out, "func "+fname+"(in *jlexer.Lexer, out *"+typ+") {")
	fmt.Fprintln(g.out, " isTopLevel := in.IsStart()")
	err := g.genTypeDecoderNoCheck(t, "*out", fieldTags{}, 1)
	if err != nil {
		return err
	}
	fmt.Fprintln(g.out, "  if isTopLevel {")
	fmt.Fprintln(g.out, "    in.Consumed()")
	fmt.Fprintln(g.out, "  }")
	fmt.Fprintln(g.out, "}")

	return nil
}

func (g *Generator) genStructDecoder(t types.Type) error {
	v, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("cannot generate decoder for %v, not a struct type", t)
	}

	fname := g.getDecoderName(t)
	typ := g.getType(t)

	fmt.Fprintln(g.out, "func "+fname+"(in *jlexer.Lexer, out *"+typ+") {")
	fmt.Fprintln(g.out, "  isTopLevel := in.IsStart()")
	fmt.Fprintln(g.out, "  if in.IsNull() {")
	fmt.Fprintln(g.out, "    if isTopLevel {")
	fmt.Fprintln(g.out, "      in.Consumed()")
	fmt.Fprintln(g.out, "    }")
	fmt.Fprintln(g.out, "    in.Skip()")
	fmt.Fprintln(g.out, "    return")
	fmt.Fprintln(g.out, "  }")

	// Init embedded pointer fields.
	for i := 0; i < v.NumFields(); i++ {
		f := v.Field(i)
		if !f.Anonymous() {
			continue
		}
		p, ok := f.Type().(*types.Pointer)
		if !ok {
			continue
		}
		fmt.Fprintln(g.out, "  out."+f.Name()+" = new("+g.getType(p.Elem())+")")
	}

	fs, err := getStructFields(v)
	if err != nil {
		return fmt.Errorf("cannot generate decoder for %v: %v", t, err)
	}

	for _, f := range fs {
		g.genRequiredFieldSet(v, f)
	}

	fmt.Fprintln(g.out, "  in.Delim('{')")
	fmt.Fprintln(g.out, "  for !in.IsDelim('}') {")
	fmt.Fprintln(g.out, "    key := in.UnsafeString()")
	fmt.Fprintln(g.out, "    in.WantColon()")
	fmt.Fprintln(g.out, "    if in.IsNull() {")
	fmt.Fprintln(g.out, "       in.Skip()")
	fmt.Fprintln(g.out, "       in.WantComma()")
	fmt.Fprintln(g.out, "       continue")
	fmt.Fprintln(g.out, "    }")

	fmt.Fprintln(g.out, "    switch key {")
	for _, f := range fs {
		if err := g.genStructFieldDecoder(v, f); err != nil {
			return err
		}
	}

	fmt.Fprintln(g.out, "    default:")
	if g.disallowUnknownFields {
		fmt.Fprintln(g.out, `      in.AddError(&jlexer.LexerError{
          Offset: in.GetPos(),
          Reason: "unknown field",
          Data: key,
      })`)
	} else {
		fmt.Fprintln(g.out, "      in.SkipRecursive()")
	}
	fmt.Fprintln(g.out, "    }")
	fmt.Fprintln(g.out, "    in.WantComma()")
	fmt.Fprintln(g.out, "  }")
	fmt.Fprintln(g.out, "  in.Delim('}')")
	fmt.Fprintln(g.out, "  if isTopLevel {")
	fmt.Fprintln(g.out, "    in.Consumed()")
	fmt.Fprintln(g.out, "  }")

	for _, f := range fs {
		g.genRequiredFieldCheck(v, f)
	}

	fmt.Fprintln(g.out, "}")

	return nil
}

func (g *Generator) genStructUnmarshaler(t types.Type) error {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array, *types.Map, *types.Struct:
	default:
		return fmt.Errorf("cannot generate decoder for %v, not a struct/slice/array/map type", t)
	}

	fname := g.getDecoderName(t)
	typ := g.getType(t)

	if !g.noStdMarshalers {
		fmt.Fprintln(g.out, "// UnmarshalJSON supports json.Unmarshaler interface")
		fmt.Fprintln(g.out, "func (v *"+typ+") UnmarshalJSON(data []byte) error {")
		fmt.Fprintln(g.out, "  r := jlexer.Lexer{Data: data}")
		fmt.Fprintln(g.out, "  "+fname+"(&r, v)")
		fmt.Fprintln(g.out, "  return r.Error()")
		fmt.Fprintln(g.out, "}")
	}

	fmt.Fprintln(g.out, "// UnmarshalEasyJSON supports easyjson.Unmarshaler interface")
	fmt.Fprintln(g.out, "func (v *"+typ+") UnmarshalEasyJSON(l *jlexer.Lexer) {")
	fmt.Fprintln(g.out, "  "+fname+"(l, v)")
	fmt.Fprintln(g.out, "}")

	return nil
}
