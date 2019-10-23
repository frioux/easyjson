package gen

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"
)

func (g *Generator) getEncoderName(t types.Type) string {
	return g.functionName("encode", t)
}

var primitiveEncoders = map[types.BasicKind]string{
	types.String:  "out.String(string(%v))",
	types.Bool:    "out.Bool(bool(%v))",
	types.Int:     "out.Int(int(%v))",
	types.Int8:    "out.Int8(int8(%v))",
	types.Int16:   "out.Int16(int16(%v))",
	types.Int32:   "out.Int32(int32(%v))",
	types.Int64:   "out.Int64(int64(%v))",
	types.Uint:    "out.Uint(uint(%v))",
	types.Uint8:   "out.Uint8(uint8(%v))",
	types.Uint16:  "out.Uint16(uint16(%v))",
	types.Uint32:  "out.Uint32(uint32(%v))",
	types.Uint64:  "out.Uint64(uint64(%v))",
	types.Float32: "out.Float32(float32(%v))",
	types.Float64: "out.Float64(float64(%v))",
}

var primitiveStringEncoders = map[types.BasicKind]string{
	types.String:  "out.String(string(%v))",
	types.Int:     "out.IntStr(int(%v))",
	types.Int8:    "out.Int8Str(int8(%v))",
	types.Int16:   "out.Int16Str(int16(%v))",
	types.Int32:   "out.Int32Str(int32(%v))",
	types.Int64:   "out.Int64Str(int64(%v))",
	types.Uint:    "out.UintStr(uint(%v))",
	types.Uint8:   "out.Uint8Str(uint8(%v))",
	types.Uint16:  "out.Uint16Str(uint16(%v))",
	types.Uint32:  "out.Uint32Str(uint32(%v))",
	types.Uint64:  "out.Uint64Str(uint64(%v))",
	types.Uintptr: "out.UintptrStr(uintptr(%v))",
	types.Float32: "out.Float32Str(float32(%v))",
	types.Float64: "out.Float64Str(float64(%v))",
}

// fieldTags contains parsed version of json struct field tags.
type fieldTags struct {
	name string

	omit        bool
	omitEmpty   bool
	noOmitEmpty bool
	asString    bool
	required    bool
}

// parseFieldTags parses the json field tag into a structure.
func parseFieldTags(s string) fieldTags {
	var ret fieldTags

	for i, s := range strings.Split(reflect.StructTag(s).Get("json"), ",") {
		switch {
		case i == 0 && s == "-":
			ret.omit = true
		case i == 0:
			ret.name = s
		case s == "omitempty":
			ret.omitEmpty = true
		case s == "!omitempty":
			ret.noOmitEmpty = true
		case s == "string":
			ret.asString = true
		case s == "required":
			ret.required = true
		}
	}

	return ret
}

// set in decoder.go
var easyjsonMarshaler, jsonMarshaler, encodingTextMarshaler *types.Interface

// genTypeEncoder generates code that encodes in of type t into the writer, but uses marshaler interface if implemented by t.
func (g *Generator) genTypeEncoder(t types.Type, in string, tags fieldTags, indent int, assumeNonEmpty bool) error {
	ws := strings.Repeat("  ", indent)

	p := types.NewPointer(t)
	if implements(p, easyjsonMarshaler) {
		fmt.Fprintln(g.out, ws+"("+in+").MarshalEasyJSON(out)")
		return nil
	}

	if implements(p, jsonMarshaler) {
		fmt.Fprintln(g.out, ws+"out.Raw( ("+in+").MarshalJSON() )")
		return nil
	}

	if implements(p, encodingTextMarshaler) {
		fmt.Fprintln(g.out, ws+"out.RawText( ("+in+").MarshalText() )")
		return nil
	}

	err := g.genTypeEncoderNoCheck(t, in, tags, indent, assumeNonEmpty)
	return err
}

// returns true of the type t implements one of the custom marshaler interfaces
func hasCustomMarshaler(t types.Type) bool {
	t = types.NewPointer(t)
	return implements(t, easyjsonMarshaler) ||
		implements(t, jsonMarshaler) ||
		implements(t, encodingTextMarshaler)
}

// genTypeEncoderNoCheck generates code that encodes in of type t into the writer.
func (g *Generator) genTypeEncoderNoCheck(t types.Type, in string, tags fieldTags, indent int, assumeNonEmpty bool) error {
	ws := strings.Repeat("  ", indent)

	if b, ok := t.Underlying().(*types.Basic); ok {
		// Check whether type is primitive, needs to be done after interface check.
		if enc := primitiveStringEncoders[b.Kind()]; enc != "" && tags.asString {
			fmt.Fprintf(g.out, ws+enc+"\n", in)
			return nil
		}

		if enc := primitiveEncoders[b.Kind()]; enc != "" {
			fmt.Fprintf(g.out, ws+enc+"\n", in)
			return nil
		}
	}

	switch v := t.Underlying().(type) {
	case *types.Slice:
		elem := v.Elem()
		iVar := g.uniqueVarName()
		vVar := g.uniqueVarName()

		if b, ok := elem.(*types.Basic); ok && b.Kind() == types.Uint8 {
			fmt.Fprintln(g.out, ws+"out.Base64Bytes("+in+")")
		} else {
			if !assumeNonEmpty {
				fmt.Fprintln(g.out, ws+"if "+in+" == nil && (out.Flags & jwriter.NilSliceAsEmpty) == 0 {")
				fmt.Fprintln(g.out, ws+`  out.RawString("null")`)
				fmt.Fprintln(g.out, ws+"} else {")
			} else {
				fmt.Fprintln(g.out, ws+"{")
			}
			fmt.Fprintln(g.out, ws+"  out.RawByte('[')")
			fmt.Fprintln(g.out, ws+"  for "+iVar+", "+vVar+" := range "+in+" {")
			fmt.Fprintln(g.out, ws+"    if "+iVar+" > 0 {")
			fmt.Fprintln(g.out, ws+"      out.RawByte(',')")
			fmt.Fprintln(g.out, ws+"    }")

			if err := g.genTypeEncoder(elem, vVar, tags, indent+2, false); err != nil {
				return err
			}

			fmt.Fprintln(g.out, ws+"  }")
			fmt.Fprintln(g.out, ws+"  out.RawByte(']')")
			fmt.Fprintln(g.out, ws+"}")
		}

	case *types.Array:
		elem := v.Elem()
		iVar := g.uniqueVarName()

		if b, ok := elem.(*types.Basic); ok && b.Kind() == types.Uint8 {
			fmt.Fprintln(g.out, ws+"out.Base64Bytes("+in+"[:])")
		} else {
			fmt.Fprintln(g.out, ws+"out.RawByte('[')")
			fmt.Fprintln(g.out, ws+"for "+iVar+" := range "+in+" {")
			fmt.Fprintln(g.out, ws+"  if "+iVar+" > 0 {")
			fmt.Fprintln(g.out, ws+"    out.RawByte(',')")
			fmt.Fprintln(g.out, ws+"  }")

			if err := g.genTypeEncoder(elem, "("+in+")["+iVar+"]", tags, indent+1, false); err != nil {
				return err
			}

			fmt.Fprintln(g.out, ws+"}")
			fmt.Fprintln(g.out, ws+"out.RawByte(']')")
		}

	case *types.Struct:
		enc := g.getEncoderName(t)
		g.addType(t)

		fmt.Fprintln(g.out, ws+enc+"(out, "+in+")")

	case *types.Pointer:
		if !assumeNonEmpty {
			fmt.Fprintln(g.out, ws+"if "+in+" == nil {")
			fmt.Fprintln(g.out, ws+`  out.RawString("null")`)
			fmt.Fprintln(g.out, ws+"} else {")
		}

		if err := g.genTypeEncoder(v.Elem(), "*"+in, tags, indent+1, false); err != nil {
			return err
		}

		if !assumeNonEmpty {
			fmt.Fprintln(g.out, ws+"}")
		}

	case *types.Map:
		key := v.Key()
		b, ok := key.(*types.Basic)
		if !ok {
			return fmt.Errorf("map type %v not supported: only string and integer keys and types implementing json.Unmarshaler are allowed", key)
		}
		keyEnc, ok := primitiveStringEncoders[b.Kind()]
		if !ok && !hasCustomMarshaler(key) {
			return fmt.Errorf("map key type %v not supported: only string and integer keys and types implementing Marshaler interfaces are allowed", key)
		} // else assume the caller knows what they are doing and that the custom marshaler performs the translation from the key type to a string or integer
		tmpVar := g.uniqueVarName()

		if !assumeNonEmpty {
			fmt.Fprintln(g.out, ws+"if "+in+" == nil && (out.Flags & jwriter.NilMapAsEmpty) == 0 {")
			fmt.Fprintln(g.out, ws+"  out.RawString(`null`)")
			fmt.Fprintln(g.out, ws+"} else {")
		} else {
			fmt.Fprintln(g.out, ws+"{")
		}
		fmt.Fprintln(g.out, ws+"  out.RawByte('{')")
		fmt.Fprintln(g.out, ws+"  "+tmpVar+"First := true")
		fmt.Fprintln(g.out, ws+"  for "+tmpVar+"Name, "+tmpVar+"Value := range "+in+" {")
		fmt.Fprintln(g.out, ws+"    if "+tmpVar+"First { "+tmpVar+"First = false } else { out.RawByte(',') }")

		// NOTE: extra check for TextMarshaler. It overrides default methods.
		if implements(types.NewPointer(key), encodingTextMarshaler) {
			fmt.Fprintln(g.out, ws+"    "+fmt.Sprintf("out.RawText(("+tmpVar+"Name).MarshalText()"+")"))
		} else if keyEnc != "" {
			fmt.Fprintln(g.out, ws+"    "+fmt.Sprintf(keyEnc, tmpVar+"Name"))
		} else {
			if err := g.genTypeEncoder(key, tmpVar+"Name", tags, indent+2, false); err != nil {
				return err
			}
		}

		fmt.Fprintln(g.out, ws+"    out.RawByte(':')")

		if err := g.genTypeEncoder(v.Elem(), tmpVar+"Value", tags, indent+2, false); err != nil {
			return err
		}

		fmt.Fprintln(g.out, ws+"  }")
		fmt.Fprintln(g.out, ws+"  out.RawByte('}')")
		fmt.Fprintln(g.out, ws+"}")

	case *types.Interface:
		if v.NumMethods() != 0 {
			return fmt.Errorf("interface type %v not supported: only interface{} is allowed", t)
		}
		fmt.Fprintln(g.out, ws+"if m, ok := "+in+".(easyjson.Marshaler); ok {")
		fmt.Fprintln(g.out, ws+"  m.MarshalEasyJSON(out)")
		fmt.Fprintln(g.out, ws+"} else if m, ok := "+in+".(json.Marshaler); ok {")
		fmt.Fprintln(g.out, ws+"  out.Raw(m.MarshalJSON())")
		fmt.Fprintln(g.out, ws+"} else {")
		fmt.Fprintln(g.out, ws+"  out.Raw(json.Marshal("+in+"))")
		fmt.Fprintln(g.out, ws+"}")

	default:
		return fmt.Errorf("don't know how to encode %v", t)
	}
	return nil
}

// set in decoder.go
var easyjsonOptional *types.Interface

func (g *Generator) notEmptyCheck(t types.Type, v string) string {
	if implements(types.NewPointer(t), easyjsonOptional) {
		return "(" + v + ").IsDefined()"
	}

	switch t1 := t.(type) {
	case *types.Slice, *types.Map:
		return "len(" + v + ") != 0"
	case *types.Interface, *types.Pointer:
		return v + " != nil"
	case *types.Basic:
		{
			switch t1.Kind() {
			case types.Bool:
				return v
			case types.String:
				return v + ` != ""`
			case types.Float32, types.Float64,
				types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
				types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:

				return v + " != 0"
			}
		}
	}
	// note: Array types don't have a useful empty value
	return "true"
}

func (g *Generator) genStructFieldEncoder(t *types.Struct, i int, firstCondition bool) (bool, error) {
	jsonName := g.fieldNamer.GetJSONFieldName(t, i)
	f := t.Field(i)
	tags := parseFieldTags(t.Tag(i))

	if tags.omit {
		return firstCondition, nil
	}

	toggleFirstCondition := firstCondition

	noOmitEmpty := (!tags.omitEmpty && !g.omitEmpty) || tags.noOmitEmpty
	if noOmitEmpty {
		fmt.Fprintln(g.out, "  {")
		toggleFirstCondition = false
	} else {
		fmt.Fprintln(g.out, "  if", g.notEmptyCheck(f.Type(), "in."+f.Name()), "{")
		// can be any in runtime, so toggleFirstCondition stay as is
	}

	if firstCondition {
		fmt.Fprintf(g.out, "    const prefix string = %q\n", ","+strconv.Quote(jsonName)+":")
		if i == 0 {
			if !noOmitEmpty {
				fmt.Fprintln(g.out, "      first = false")
			}
			fmt.Fprintln(g.out, "      out.RawString(prefix[1:])")
		} else {
			fmt.Fprintln(g.out, "    if first {")
			fmt.Fprintln(g.out, "      first = false")
			fmt.Fprintln(g.out, "      out.RawString(prefix[1:])")
			fmt.Fprintln(g.out, "    } else {")
			fmt.Fprintln(g.out, "      out.RawString(prefix)")
			fmt.Fprintln(g.out, "    }")
		}
	} else {
		fmt.Fprintf(g.out, "    const prefix string = %q\n", ","+strconv.Quote(jsonName)+":")
		fmt.Fprintln(g.out, "    out.RawString(prefix)")
	}

	if err := g.genTypeEncoder(f.Type(), "in."+f.Name(), tags, 2, !noOmitEmpty); err != nil {
		return toggleFirstCondition, err
	}
	fmt.Fprintln(g.out, "  }")
	return toggleFirstCondition, nil
}

func (g *Generator) genEncoder(t types.Type) error {
	switch t.(type) {
	case *types.Slice, *types.Array, *types.Map:
		return g.genSliceArrayMapEncoder(t)
	default:
		return g.genStructEncoder(t)
	}
}

func (g *Generator) genSliceArrayMapEncoder(t types.Type) error {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array, *types.Map:
	default:
		return fmt.Errorf("cannot generate encoder for %v, not a slice/array/map type", t)
	}

	fname := g.getEncoderName(t)
	typ := g.getType(t)

	fmt.Fprintln(g.out, "func "+fname+"(out *jwriter.Writer, in "+typ+") {")
	err := g.genTypeEncoderNoCheck(t, "in", fieldTags{}, 1, false)
	if err != nil {
		return err
	}
	fmt.Fprintln(g.out, "}")
	return nil
}

func (g *Generator) genStructEncoder(t types.Type) error {
	s, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("cannot generate encoder for %v, not a struct type", t)
	}

	fname := g.getEncoderName(t)
	typ := g.getType(t)

	fmt.Fprintln(g.out, "func "+fname+"(out *jwriter.Writer, in "+typ+") {")
	fmt.Fprintln(g.out, "  out.RawByte('{')")
	fmt.Fprintln(g.out, "  first := true")
	fmt.Fprintln(g.out, "  _ = first")

	fs, err := getStructFields(s)
	if err != nil {
		return fmt.Errorf("cannot generate encoder for %v: %v", t, err)
	}

	firstCondition := true
	for _, i := range fs {
		firstCondition, err = g.genStructFieldEncoder(s, i, firstCondition)

		if err != nil {
			return err
		}
	}

	fmt.Fprintln(g.out, "  out.RawByte('}')")
	fmt.Fprintln(g.out, "}")

	return nil
}

func (g *Generator) genStructMarshaler(t types.Type) error {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array, *types.Map, *types.Struct:
	default:
		return fmt.Errorf("cannot generate encoder for %v, not a struct/slice/array/map type", t)
	}

	fname := g.getEncoderName(t)
	typ := g.getType(t)

	if !g.noStdMarshalers {
		fmt.Fprintln(g.out, "// MarshalJSON supports json.Marshaler interface")
		fmt.Fprintln(g.out, "func (v "+typ+") MarshalJSON() ([]byte, error) {")
		fmt.Fprintln(g.out, "  w := jwriter.Writer{}")
		fmt.Fprintln(g.out, "  "+fname+"(&w, v)")
		fmt.Fprintln(g.out, "  return w.Buffer.BuildBytes(), w.Error")
		fmt.Fprintln(g.out, "}")
	}

	fmt.Fprintln(g.out, "// MarshalEasyJSON supports easyjson.Marshaler interface")
	fmt.Fprintln(g.out, "func (v "+typ+") MarshalEasyJSON(w *jwriter.Writer) {")
	fmt.Fprintln(g.out, "  "+fname+"(w, v)")
	fmt.Fprintln(g.out, "}")

	return nil
}
