package graphqljson

import (
	"bytes"
	jsonv1 "encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"encoding/json/jsontext"
	"encoding/json/v2"

	"github.com/99designs/gqlgen/graphql"
)

var jsonRawMessageType = reflect.TypeOf(jsonv1.RawMessage{})

const (
	kindString = jsontext.Kind('"')
	kindNumber = jsontext.Kind('0')
)

// Reference: https://blog.gopheracademy.com/advent-2017/custom-json-unmarshaler-for-graphql-client/

// UnmarshalData parses the JSON-encoded GraphQL response data and stores
// the result in the GraphQL query data structure pointed to by v.
//
// The implementation is created on top of the JSON tokenizer available
// in "encoding/json/jsontext".Decoder.
func UnmarshalData(data jsontext.Value, v any) error {
	d := newDecoder(bytes.NewReader(data))
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("decode graphql data: %w", err)
	}

	tok, err := d.jsonDecoder.ReadToken()
	if errors.Is(err, io.EOF) {
		// Expect to get io.EOF. There shouldn't be any more
		// tokens left after we've decoded v successfully.
		return nil
	} else if err == nil {
		return fmt.Errorf("invalid token '%v' after top-level value (at byte offset %d)", tok, d.jsonDecoder.InputOffset())
	}

	return fmt.Errorf("invalid token '%v' after top-level value (at byte offset %d)", tok, d.jsonDecoder.InputOffset())
}

// Decoder is a JSON Decoder that performs custom unmarshaling behavior
// for GraphQL query data structures. It's implemented on top of a JSON tokenizer.
type Decoder struct {
	jsonDecoder *jsontext.Decoder

	// Stack of what part of input JSON we're in the middle of - objects, arrays.
	parseState []jsontext.Kind

	// Stacks of values where to unmarshal.
	// The top of each stack is the reflect.Value where to unmarshal next JSON value.
	//
	// The reason there's more than one stack is because we might be unmarshaling
	// a single JSON value into multiple GraphQL fragments or embedded structs, so
	// we keep track of them all.
	vs [][]reflect.Value
}

func newDecoder(r io.Reader) *Decoder {
	jsonDecoder := jsontext.NewDecoder(r)

	return &Decoder{
		jsonDecoder: jsonDecoder,
	}
}

// Decode decodes a single JSON value from d.tokenizer into v.
func (d *Decoder) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("cannot decode into non-pointer %T", v)
	}

	d.vs = [][]reflect.Value{{rv.Elem()}}
	if err := d.decode(); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}

// decode decodes a single JSON value from d.tokenizer into d.vs.
func (d *Decoder) decode() error {
	readToken := func(context string) (jsontext.Token, error) {
		tok, err := d.jsonDecoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return jsontext.Token{}, errors.New("unexpected end of JSON input")
		}
		if err != nil {
			return jsontext.Token{}, fmt.Errorf("%s: %w", context, err)
		}
		return tok, nil
	}

	for len(d.vs) > 0 {
		state := d.state()
		var tok jsontext.Token
		var err error

		switch state {
		case jsontext.BeginObject.Kind():
			nextKind := d.jsonDecoder.PeekKind()
			if nextKind == jsontext.EndObject.Kind() {
				tok, err = readToken("read json token")
				if err != nil {
					return err
				}
			} else {
				tok, err = readToken("read json token")
				if err != nil {
					return err
				}

				key := tok.String()
				var found bool

				for i := range d.vs {
					current := d.vs[i][len(d.vs[i])-1]
					candidate := derefValue(current)

					var f reflect.Value
					if candidate.IsValid() && candidate.Kind() == reflect.Struct {
						f = fieldByGraphQLName(candidate, key)
						if f.IsValid() {
							found = true
						}
					}

					d.vs[i] = append(d.vs[i], f)
				}

				if !found {
					return fmt.Errorf("struct field for %q doesn't exist in any of %v places to unmarshal (at byte offset %d)", key, len(d.vs), d.jsonDecoder.InputOffset())
				}

				nextKind = d.jsonDecoder.PeekKind()
				handled, handleErr := d.handleCompositeSpecialType(nextKind)
				if handleErr != nil {
					return handleErr
				}
				if handled {
					continue
				}

				tok, err = readToken("read field value token")
				if err != nil {
					return err
				}
			}

		case jsontext.BeginArray.Kind():
			nextKind := d.jsonDecoder.PeekKind()
			if nextKind == jsontext.EndArray.Kind() {
				tok, err = readToken("read json token")
				if err != nil {
					return err
				}
			} else {
				someSliceExist := false

				for i := range d.vs {
					base := d.vs[i][len(d.vs[i])-1]
					target, ok := ensureValue(base)
					if !ok {
						d.vs[i] = append(d.vs[i], reflect.Value{})
						continue
					}

					var f reflect.Value
					if target.Kind() == reflect.Slice {
						target.Set(reflect.Append(target, reflect.Zero(target.Type().Elem())))
						f = target.Index(target.Len() - 1)
						someSliceExist = true
					}

					d.vs[i] = append(d.vs[i], f)
				}

				if !someSliceExist {
					return fmt.Errorf("slice doesn't exist in any of %v places to unmarshal (at byte offset %d)", len(d.vs), d.jsonDecoder.InputOffset())
				}

				handled, handleErr := d.handleCompositeSpecialType(nextKind)
				if handleErr != nil {
					return handleErr
				}
				if handled {
					continue
				}

				tok, err = readToken("read json token")
				if err != nil {
					return err
				}
			}

		default:
			handled, handleErr := d.handleCompositeSpecialType(d.jsonDecoder.PeekKind())
			if handleErr != nil {
				return handleErr
			}
			if handled {
				continue
			}

			tok, err = readToken("read json token")
			if err != nil {
				return err
			}
		}

		switch tok.Kind() {
		case jsontext.Null.Kind():
			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if !v.CanSet() {
					continue
				}
				v.Set(reflect.Zero(v.Type()))
			}
			d.popAllVs()
			continue

		case kindString, jsontext.True.Kind(), jsontext.False.Kind(), kindNumber:
			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if !v.IsValid() {
					continue
				}

				target, assigned := ensureValue(v)
				if !assigned {
					continue
				}

				var unmarshaler graphql.Unmarshaler
				implements := false
				if target.CanAddr() {
					unmarshaler, implements = target.Addr().Interface().(graphql.Unmarshaler)
				} else if target.CanInterface() {
					unmarshaler, implements = target.Interface().(graphql.Unmarshaler)
				}

				if implements {
					var value any
					switch tok.Kind() {
					case kindString:
						value = tok.String()
					case jsontext.True.Kind(), jsontext.False.Kind():
						value = tok.Bool()
					case kindNumber:
						if intVal := tok.Int(); intVal == tok.Int() {
							value = intVal
						} else {
							value = tok.Float()
						}
					}

					if err := unmarshaler.UnmarshalGQL(value); err != nil {
						return fmt.Errorf("unmarshal gql error: %w", err)
					}
				} else {
					if err := unmarshalValue(tok, target); err != nil {
						return fmt.Errorf("unmarshal value: %w", err)
					}
				}
			}

			d.popAllVs()

		case jsontext.BeginObject.Kind(), jsontext.BeginArray.Kind():
			isArray := tok.Kind() == jsontext.BeginArray.Kind()
			if isArray {
				d.pushState(tok.Kind())
				for i := range d.vs {
					base := d.vs[i][len(d.vs[i])-1]
					target, ok := ensureValue(base)
					if !ok {
						continue
					}
					if target.Kind() != reflect.Slice {
						continue
					}
					target.Set(reflect.MakeSlice(target.Type(), 0, 0))
				}
			} else {
				d.pushState(tok.Kind())
				frontier := make([]reflect.Value, len(d.vs))
				for i := range d.vs {
					base := d.vs[i][len(d.vs[i])-1]
					_, _ = ensureValue(base)
					frontier[i] = derefValue(base)
				}
				for len(frontier) > 0 {
					v := frontier[0]
					frontier = frontier[1:]
					v = derefValue(v)
					if !v.IsValid() || v.Kind() != reflect.Struct {
						continue
					}
					for i := range v.NumField() {
						if isGraphQLFragment(v.Type().Field(i)) || v.Type().Field(i).Anonymous {
							field := v.Field(i)
							d.vs = append(d.vs, []reflect.Value{field})
							frontier = append(frontier, field)
						}
					}
				}
			}

		case jsontext.EndObject.Kind(), jsontext.EndArray.Kind():
			d.popAllVs()
			d.popState()

		default:
			return fmt.Errorf("unexpected token in JSON input (at byte offset %d)", d.jsonDecoder.InputOffset())
		}
	}

	return nil
}

func (d *Decoder) handleCompositeSpecialType(peek jsontext.Kind) (bool, error) {
	if peek != jsontext.BeginObject.Kind() && peek != jsontext.BeginArray.Kind() {
		return false, nil
	}

	hasSpecialType := false
	for i := range d.vs {
		if len(d.vs[i]) == 0 {
			continue
		}

		candidate := derefValue(d.vs[i][len(d.vs[i])-1])
		if !candidate.IsValid() {
			continue
		}

		typ := candidate.Type()
		if typ == jsonRawMessageType || typ.Kind() == reflect.Map {
			hasSpecialType = true
			break
		}
	}

	if !hasSpecialType {
		return false, nil
	}

	value, err := d.jsonDecoder.ReadValue()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, errors.New("unexpected end of JSON input")
		}
		return false, fmt.Errorf("read composite value: %w", err)
	}

	clone := value.Clone()
	if err := clone.Format(); err != nil {
		return false, fmt.Errorf("format composite value: %w", err)
	}
	bytes := []byte(clone)

	for i := range d.vs {
		v := d.vs[i][len(d.vs[i])-1]
		if !v.IsValid() {
			continue
		}

		target, ok := ensureValue(v)
		if !ok {
			continue
		}

		switch {
		case target.Type() == jsonRawMessageType:
			target.SetBytes(bytes)
		case target.Kind() == reflect.Map:
			if target.IsNil() {
				target.Set(reflect.MakeMap(target.Type()))
			}
			if err := json.Unmarshal(bytes, target.Addr().Interface()); err != nil {
				return false, fmt.Errorf("unmarshal into map: %w", err)
			}
		}
	}

	d.popAllVs()
	return true, nil
}

// pushState pushes a new parse state s onto the stack.
func (d *Decoder) pushState(s jsontext.Kind) {
	d.parseState = append(d.parseState, s)
}

// popState pops a parse state (already obtained) off the stack.
// The stack must be non-empty.
func (d *Decoder) popState() {
	d.parseState = d.parseState[:len(d.parseState)-1]
}

// state reports the parse state on top of stack, or 0 if empty.
func (d *Decoder) state() jsontext.Kind {
	if len(d.parseState) == 0 {
		return 0
	}

	return d.parseState[len(d.parseState)-1]
}

// popAllVs pops from all d.vs stacks, keeping only non-empty ones.
func (d *Decoder) popAllVs() {
	var nonEmpty [][]reflect.Value

	for i := range d.vs {
		d.vs[i] = d.vs[i][:len(d.vs[i])-1]
		if len(d.vs[i]) > 0 {
			nonEmpty = append(nonEmpty, d.vs[i])
		}
	}

	d.vs = nonEmpty
}

func derefValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}

	if v.Kind() != reflect.Ptr {
		return v
	}

	if v.IsNil() {
		return reflect.Zero(v.Type().Elem())
	}

	return v.Elem()
}

func ensureValue(v reflect.Value) (reflect.Value, bool) {
	if !v.IsValid() {
		return reflect.Value{}, false
	}

	if v.Kind() != reflect.Ptr {
		return v, true
	}

	if v.IsNil() {
		if !v.CanSet() {
			return reflect.Value{}, false
		}
		v.Set(reflect.New(v.Type().Elem()))
	}

	return v.Elem(), true
}

// fieldByGraphQLName returns an exported struct field of struct v
// that matches GraphQL name, or invalid reflect.Value if none found.
func fieldByGraphQLName(v reflect.Value, name string) reflect.Value {
	for i := range v.NumField() {
		if v.Type().Field(i).PkgPath != "" {
			// Skip unexported field.
			continue
		}

		if hasGraphQLName(v.Type().Field(i), name) {
			return v.Field(i)
		}
	}

	return reflect.Value{}
}

// hasGraphQLName reports whether struct field f has GraphQL name.
func hasGraphQLName(f reflect.StructField, name string) bool {
	// First check graphql tag
	value, ok := f.Tag.Lookup("graphql")
	if ok {
		value = strings.TrimSpace(value) // TODO: Parse better.
		if strings.HasPrefix(value, "...") {
			// GraphQL fragment. It doesn't have a name.
			return false
		}

		if i := strings.Index(value, "("); i != -1 {
			value = value[:i]
		}

		if i := strings.Index(value, ":"); i != -1 {
			value = value[:i]
		}

		return strings.TrimSpace(value) == name
	}

	// If no graphql tag, check json tag
	jsonValue, ok := f.Tag.Lookup("json")
	if ok {
		jsonValue = strings.TrimSpace(jsonValue)
		// Handle json tag options (e.g., "name,omitempty")
		if i := strings.Index(jsonValue, ","); i != -1 {
			jsonValue = jsonValue[:i]
		}
		if jsonValue == name {
			return true
		}
	}

	// Fall back to field name comparison
	// TODO: caseconv package is relatively slow. Optimize it, then consider using it here.
	// return caseconv.MixedCapsToLowerCamelCase(f.Name) == name
	return strings.EqualFold(f.Name, name)
}

// isGraphQLFragment reports whether struct field f is a GraphQL fragment.
func isGraphQLFragment(f reflect.StructField) bool {
	value, ok := f.Tag.Lookup("graphql")
	if !ok {
		return false
	}

	value = strings.TrimSpace(value) // TODO: Parse better.

	return strings.HasPrefix(value, "...")
}

// unmarshalValue unmarshals JSON value into v.
// v must be addressable and not obtained by the use of unexported
// struct fields, otherwise unmarshalValue will panic.
func unmarshalValue(value jsontext.Token, v reflect.Value) error {
	// For primitive types, set values directly without marshaling/unmarshaling
	switch value.Kind() {
	case kindString:
		str := value.String()
		if v.Kind() == reflect.String {
			v.SetString(str)
			return nil
		}
		// For other string-compatible types, use jsonv2
		b, err := json.Marshal(str)
		if err != nil {
			return fmt.Errorf("marshal string: %w", err)
		}
		if err := json.Unmarshal(b, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal string: %w", err)
		}
		return nil
	case jsontext.True.Kind(), jsontext.False.Kind():
		b := value.Bool()
		if v.Kind() == reflect.Bool {
			v.SetBool(b)
			return nil
		}
		// For other bool-compatible types, use jsonv2
		bytes, err := json.Marshal(b)
		if err != nil {
			return fmt.Errorf("marshal bool: %w", err)
		}
		if err := json.Unmarshal(bytes, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal bool: %w", err)
		}
		return nil
	case kindNumber:
		// For numeric types, try direct setting first
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v.SetInt(value.Int())
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if value.Int() < 0 {
				return fmt.Errorf("cannot set negative value to unsigned type")
			}
			v.SetUint(uint64(value.Int()))
			return nil
		case reflect.Float32, reflect.Float64:
			v.SetFloat(value.Float())
			return nil
		}
		// For other numeric-compatible types, use jsonv2
		var val any
		intVal := value.Int()
		floatVal := value.Float()
		// Check if the float value is exactly representable as an integer
		if floatVal == float64(intVal) && !math.IsInf(floatVal, 0) && !math.IsNaN(floatVal) {
			val = intVal
		} else {
			val = floatVal
		}
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal number: %w", err)
		}
		if err := json.Unmarshal(b, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal number: %w", err)
		}
		return nil
	case jsontext.Null.Kind():
		v.Set(reflect.Zero(v.Type()))
		return nil
	default:
		return fmt.Errorf("unexpected token kind: %v", value.Kind())
	}
}
