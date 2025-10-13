package graphqljson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/99designs/gqlgen/graphql"
)

var jsonRawMessageType = reflect.TypeOf(json.RawMessage{})

// Reference: https://blog.gopheracademy.com/advent-2017/custom-json-unmarshaler-for-graphql-client/

// UnmarshalData parses the JSON-encoded GraphQL response data and stores
// the result in the GraphQL query data structure pointed to by v.
//
// The implementation is created on top of the JSON tokenizer available
// in "encoding/json".Decoder.
func UnmarshalData(data jsontext.Value, v any) error {
	d := newDecoder(bytes.NewReader(data))
	if err := d.Decode(v); err != nil {
		return fmt.Errorf(": %w", err)
	}

	tok, err := d.jsonDecoder.ReadToken()
	if errors.Is(err, io.EOF) {
		// Expect to get io.EOF. There shouldn't be any more
		// tokens left after we've decoded v successfully.
		return nil
	} else if err == nil {
		return fmt.Errorf("invalid token '%v' after top-level value", tok)
	}

	return fmt.Errorf("invalid token '%v' after top-level value", tok)
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
	// jsonDecoder.UseNumber()

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
		return fmt.Errorf(": %w", err)
	}

	return nil
}

// decode decodes a single JSON value from d.tokenizer into d.vs.
func (d *Decoder) decode() error {
	// The loop invariant is that the top of each d.vs stack
	// is where we try to unmarshal the next JSON value we see.
	for len(d.vs) > 0 {
		tok, err := d.jsonDecoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return errors.New("unexpected end of JSON input")
		} else if err != nil {
			return fmt.Errorf(": %w", err)
		}

		switch {
		// Are we inside an object and seeing next key (rather than end of object)?
		case d.state() == '{' && tok.Kind() != '}':
			key := tok.String()

			// The last matching one is the one considered
			var matchingFieldValue *reflect.Value

			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if v.Kind() == reflect.Ptr {
					v = v.Elem()
				}

				var f reflect.Value
				if v.Kind() == reflect.Struct {
					f = fieldByGraphQLName(v, key)
					if f.IsValid() {
						matchingFieldValue = &f
					}
				}

				d.vs[i] = append(d.vs[i], f)
			}

			if matchingFieldValue == nil {
				return fmt.Errorf("struct field for %q doesn't exist in any of %v places to unmarshal", key, len(d.vs))
			}

			tok, err = d.jsonDecoder.ReadToken()
			if errors.Is(err, io.EOF) {
				return errors.New("unexpected end of JSON input")
			} else if err != nil {
				return fmt.Errorf(": %w", err)
			}

		// Are we inside an array and seeing next value (rather than end of array)?
		case d.state() == '[' && tok.Kind() != ']':
			someSliceExist := false

			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if v.Kind() == reflect.Ptr {
					v = v.Elem()
				}

				var f reflect.Value

				if v.Kind() == reflect.Slice {
					v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem()))) // v = append(v, T).
					f = v.Index(v.Len() - 1)
					someSliceExist = true
				}

				d.vs[i] = append(d.vs[i], f)
			}

			if !someSliceExist {
				return fmt.Errorf("slice doesn't exist in any of %v places to unmarshal", len(d.vs))
			}
		}

		switch tok.Kind() {
		case 'n': // null
			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if !v.CanSet() {
					// If v is not settable, skip the operation to prevent panicking.
					continue
				}

				// Set to zero value regardless of type
				v.Set(reflect.Zero(v.Type()))
			}

			d.popAllVs()

			continue
		case '"', 't', 'f', '0': // string, true, false, number
			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if !v.IsValid() {
					continue
				}

				// Initialize the pointer if it is nil
				if v.Kind() == reflect.Ptr && v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}

				// Handle both pointer and non-pointer types
				target := v
				if v.Kind() == reflect.Ptr {
					target = v.Elem()
				}

				// Check if the type of target (or its address) implements graphql.Unmarshaler
				var unmarshaler graphql.Unmarshaler

				var ok bool
				if target.CanAddr() {
					unmarshaler, ok = target.Addr().Interface().(graphql.Unmarshaler)
				} else if target.CanInterface() {
					unmarshaler, ok = target.Interface().(graphql.Unmarshaler)
				}

				if ok {
					// Get the actual value to pass to UnmarshalGQL
					var value any
					switch tok.Kind() {
					case '"':
						value = tok.String()
					case 't', 'f':
						value = tok.Bool()
					case '0':
						// For numbers, we need to determine the type
						// Try int64 first, then float64
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
					// Use the standard unmarshal method for non-custom types
					if err := unmarshalValue(tok, target); err != nil {
						return fmt.Errorf(": %w", err)
					}
				}
			}

			d.popAllVs()

		case '{', '[': // BeginObject or BeginArray
			// Check if any current value expects raw JSON (json.RawMessage or map)
			hasSpecialType := false
			isArray := tok.Kind() == '['

			for i := range d.vs {
				v := d.vs[i][len(d.vs[i])-1]
				if !v.IsValid() {
					continue
				}

				target := v
				if v.Kind() == reflect.Ptr {
					if v.IsNil() {
						v.Set(reflect.New(v.Type().Elem()))
					}
					target = v.Elem()
				}

				// Check for json.RawMessage or map
				if target.Type() == jsonRawMessageType || target.Kind() == reflect.Map {
					hasSpecialType = true
					break
				}
			}

			// If we have json.RawMessage or map, reconstruct the JSON
			if hasSpecialType {
				jsonBytes, err := d.reconstructJSON(tok.Kind())
				if err != nil {
					return fmt.Errorf("reconstruct JSON: %w", err)
				}

				// Now set the values
				for i := range d.vs {
					v := d.vs[i][len(d.vs[i])-1]
					if !v.IsValid() {
						continue
					}

					target := v
					if v.Kind() == reflect.Ptr {
						target = v.Elem()
					}

					if target.Type() == jsonRawMessageType {
						target.SetBytes(jsonBytes)
					} else if target.Kind() == reflect.Map {
						// Initialize map if needed
						if target.IsNil() {
							target.Set(reflect.MakeMap(target.Type()))
						}
						// Unmarshal into the map using jsonv2
						if err := jsonv2.Unmarshal(jsonBytes, target.Addr().Interface()); err != nil {
							return fmt.Errorf("unmarshal into map: %w", err)
						}
					}
				}

				d.popAllVs()
				continue
			}

			// Normal handling for objects and arrays
			if isArray {
				// Start of array.
				d.pushState(tok.Kind())

				for i := range d.vs {
					v := d.vs[i][len(d.vs[i])-1]
					// TODO: Confirm this is needed, write a test case.
					// if v.Kind() == reflect.Ptr && v.IsNil() {
					//	v.Set(reflect.New(v.Type().Elem())) // v = new(T).
					//}

					// Reset slice to empty (in case it had non-zero initial value).
					if v.Kind() == reflect.Ptr {
						v = v.Elem()
					}

					if v.Kind() != reflect.Slice {
						continue
					}

					v.Set(reflect.MakeSlice(v.Type(), 0, 0)) // v = make(T, 0, 0).
				}
			} else {
				// Start of object.
				d.pushState(tok.Kind())

				frontier := make([]reflect.Value, len(d.vs)) // Places to look for GraphQL fragments/embedded structs.

				for i := range d.vs {
					v := d.vs[i][len(d.vs[i])-1]
					frontier[i] = v
					// TODO: Do this recursively or not? Add a test case if needed.
					if v.Kind() == reflect.Ptr && v.IsNil() {
						v.Set(reflect.New(v.Type().Elem())) // v = new(T).
					}
				}
				// Find GraphQL fragments/embedded structs recursively, adding to frontier
				// as new ones are discovered and exploring them further.
				for len(frontier) > 0 {
					v := frontier[0]
					frontier = frontier[1:]

					if v.Kind() == reflect.Ptr {
						v = v.Elem()
					}

					if v.Kind() != reflect.Struct {
						continue
					}

					for i := range v.NumField() {
						if isGraphQLFragment(v.Type().Field(i)) || v.Type().Field(i).Anonymous {
							// Add GraphQL fragment or embedded struct.
							d.vs = append(d.vs, []reflect.Value{v.Field(i)})
							//nolint:makezero // append to slice `frontier` with non-zero initialized length
							frontier = append(frontier, v.Field(i))
						}
					}
				}
			}
		case '}', ']': // EndObject, EndArray
			// End of object or array.
			d.popAllVs()
			d.popState()
		default:
			return errors.New("unexpected token in JSON input")
		}
	}

	return nil
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

// reconstructJSON reconstructs JSON bytes from tokens for objects/arrays.
// kind should be '{' for objects or '[' for arrays.
func (d *Decoder) reconstructJSON(kind jsontext.Kind) ([]byte, error) {
	var buf bytes.Buffer

	// Write opening bracket
	if kind == '[' {
		buf.WriteByte('[')
	} else {
		buf.WriteByte('{')
	}

	depth := 1
	needComma := false
	expectingValue := false
	inObject := kind == '{'

	for depth > 0 {
		tok, err := d.jsonDecoder.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("read token: %w", err)
		}

		switch tok.Kind() {
		case '{':
			if needComma {
				buf.WriteByte(',')
			}
			buf.WriteByte('{')
			depth++
			needComma = false
			expectingValue = false
			inObject = true
		case '}':
			buf.WriteByte('}')
			depth--
			needComma = depth > 0
			expectingValue = false
		case '[':
			if needComma {
				buf.WriteByte(',')
			}
			buf.WriteByte('[')
			depth++
			needComma = false
			expectingValue = false
			inObject = false
		case ']':
			buf.WriteByte(']')
			depth--
			needComma = depth > 0
			expectingValue = false
		case '"':
			if needComma {
				buf.WriteByte(',')
			}
			// Properly encode string using jsontext
			encoded, err := jsontext.AppendQuote(nil, tok.String())
			if err != nil {
				return nil, fmt.Errorf("quote string: %w", err)
			}
			buf.Write(encoded)
			if inObject && !expectingValue {
				// This is a key, add colon
				buf.WriteByte(':')
				expectingValue = true
				needComma = false
			} else {
				// This is a value
				expectingValue = false
				needComma = true
			}
		case 't':
			if needComma {
				buf.WriteByte(',')
			}
			buf.WriteString("true")
			expectingValue = false
			needComma = true
		case 'f':
			if needComma {
				buf.WriteByte(',')
			}
			buf.WriteString("false")
			expectingValue = false
			needComma = true
		case 'n':
			if needComma {
				buf.WriteByte(',')
			}
			buf.WriteString("null")
			expectingValue = false
			needComma = true
		case '0':
			if needComma {
				buf.WriteByte(',')
			}
			// Format number
			if float64(tok.Int()) == tok.Float() {
				buf.WriteString(strconv.FormatInt(tok.Int(), 10))
			} else {
				buf.WriteString(fmt.Sprintf("%g", tok.Float()))
			}
			expectingValue = false
			needComma = true
		}
	}

	return buf.Bytes(), nil
}

// unmarshalValue unmarshals JSON value into v.
// v must be addressable and not obtained by the use of unexported
// struct fields, otherwise unmarshalValue will panic.
func unmarshalValue(value jsontext.Token, v reflect.Value) error {
	// For primitive types, set values directly without marshaling/unmarshaling
	switch value.Kind() {
	case '"':
		str := value.String()
		if v.Kind() == reflect.String {
			v.SetString(str)
			return nil
		}
		// For other string-compatible types, use jsonv2
		b, err := jsonv2.Marshal(str)
		if err != nil {
			return fmt.Errorf("marshal string: %w", err)
		}
		if err := jsonv2.Unmarshal(b, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal string: %w", err)
		}
		return nil
	case 't', 'f':
		b := value.Bool()
		if v.Kind() == reflect.Bool {
			v.SetBool(b)
			return nil
		}
		// For other bool-compatible types, use jsonv2
		bytes, err := jsonv2.Marshal(b)
		if err != nil {
			return fmt.Errorf("marshal bool: %w", err)
		}
		if err := jsonv2.Unmarshal(bytes, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal bool: %w", err)
		}
		return nil
	case '0':
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
		if intVal := value.Int(); float64(intVal) == value.Float() {
			val = intVal
		} else {
			val = value.Float()
		}
		b, err := jsonv2.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal number: %w", err)
		}
		if err := jsonv2.Unmarshal(b, v.Addr().Interface()); err != nil {
			return fmt.Errorf("unmarshal number: %w", err)
		}
		return nil
	case 'n':
		v.Set(reflect.Zero(v.Type()))
		return nil
	default:
		return fmt.Errorf("unexpected token kind: %v", value.Kind())
	}
}
