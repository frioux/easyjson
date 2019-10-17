package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
)

type bar struct {
	D string
	E int
}

type Foo struct {
	A, B string
	C    bar
}

var errUnsupported = errors.New("type not supported")

func newErrUnsupported(s string) error {
	return fmt.Errorf("%s (type=%s)", errUnsupported, s)
}

// astToReflect translates an *ast.Node (like ast.StructType) and returns one or
// more reflect.Type values.
func astToReflect(t *types.Info, x interface{}) ([]interface{}, error) {
	switch v := x.(type) {
	case *ast.Field:
		return field(t, v)
	default:
		return nil, errUnsupported
	}
}

func exprConv(i *types.Info, in ast.Expr) (reflect.Type, error) {
	t, ok := i.Types[in]
	if !ok {
		return nil, errors.New("not found")
	}

	return typeConv(i, t.Type)
}

func typeConv(i *types.Info, t types.Type) (reflect.Type, error) {
	switch v := t.(type) {
	case *types.Basic:
		switch v.Kind() {
		case types.Bool:
			return reflect.TypeOf(bool(false)), nil
		case types.Int:
			return reflect.TypeOf(int(0)), nil
		case types.Int8:
			return reflect.TypeOf(int8(0)), nil
		case types.Int16:
			return reflect.TypeOf(int16(0)), nil
		case types.Int32:
			return reflect.TypeOf(int32(0)), nil
		case types.Int64:
			return reflect.TypeOf(int64(0)), nil
		case types.Uint:
			return reflect.TypeOf(uint(0)), nil
		case types.Uint8:
			return reflect.TypeOf(uint8(0)), nil
		case types.Uint16:
			return reflect.TypeOf(uint16(0)), nil
		case types.Uint32:
			return reflect.TypeOf(uint32(0)), nil
		case types.Uint64:
			return reflect.TypeOf(uint64(0)), nil
		case types.Uintptr:
			return reflect.TypeOf(uintptr(0)), nil
		case types.Float32:
			return reflect.TypeOf(float32(0)), nil
		case types.Float64:
			return reflect.TypeOf(float64(0)), nil
		case types.Complex64:
			return reflect.TypeOf(complex64(0)), nil
		case types.Complex128:
			return reflect.TypeOf(complex128(0)), nil
		case types.String:
			return reflect.TypeOf(string("")), nil
		// UnsafePointer

		// types for untyped values
		// UntypedBool
		// UntypedInt
		// UntypedRune
		// UntypedFloat
		// UntypedComplex
		// UntypedString
		// UntypedNil

		// aliases
		// Byte
		// Rune
		default:
			return nil, newErrUnsupported(fmt.Sprintf("%T", v.Kind()))
		}
	case *types.Chan:
		return nil, newErrUnsupported("chan")
	case *types.Array:
		return nil, newErrUnsupported("array")
	// case *types.Func:
	// 	return nil, errUnsupported
	case *types.Interface:
		return nil, newErrUnsupported("interface")
	case *types.Map:
		return nil, newErrUnsupported("map")
	case *types.Pointer:
		return nil, newErrUnsupported("pointer")
	case *types.Slice:
		return nil, newErrUnsupported("slice")
	case *types.Struct:
		fields := make([]reflect.StructField, 0, v.NumFields())

		for j := 0; j < v.NumFields(); j++ {
			fieldVar := v.Field(j)
			// we don't encode/decode unexported fields
			if !fieldVar.Exported() {
				continue
			}
			t, err := typeConv(i, fieldVar.Type())
			if err != nil {
				return nil, fmt.Errorf("couldn't convert field (name=%s type=%s): %s", fieldVar.Name(), fieldVar.Type(), err)
			}
			structField := reflect.StructField{
				Name: fieldVar.Name(),
				Tag:  reflect.StructTag(v.Tag(j)),
				Type: t,
			}

			fields = append(fields, structField)
		}

		return reflect.StructOf(fields), nil
	default:
		return typeConv(i, v.Underlying())
	}
}

func field(i *types.Info, f *ast.Field) ([]interface{}, error) {
	fields := make([]interface{}, 0, len(f.Names))
	_t, err := astToReflect(i, f.Type)
	if err != nil {
		return nil, err
	}
	if len(_t) != 1 {
		return nil, errors.New("WTF how did you get non-1 types for a field type")
	}
	t, ok := _t[0].(reflect.Type)
	if !ok {
		return nil, fmt.Errorf("got non reflect.Type back: %T", _t)
	}

	for _, name := range f.Names {
		var tag reflect.StructTag
		if f.Tag != nil {
			tag = reflect.StructTag(f.Tag.Value)
		}

		fields = append(fields, reflect.StructField{
			Name: name.Name,
			Type: t,
			Tag:  tag,
		})
	}
	return fields, nil
}
