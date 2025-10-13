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

type frame struct {
	kind    jsontext.Kind
	targets []reflect.Value
}

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
	frames      []frame
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

	d.frames = []frame{{kind: 0, targets: []reflect.Value{rv.Elem()}}}
	if err := d.decode(); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	return nil
}

// decode drives the JSON tokenizer and assigns values using the frame stack.
func (d *Decoder) decode() error {
	for len(d.frames) > 0 {
		switch d.state() {
		case jsontext.BeginObject.Kind():
			if err := d.handleObject(); err != nil {
				return err
			}
			continue
		case jsontext.BeginArray.Kind():
			if err := d.handleArray(); err != nil {
				return err
			}
			continue
		}

		tok, err := d.jsonDecoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return errors.New("unexpected end of JSON input")
		}
		if err != nil {
			return fmt.Errorf("read json token: %w", err)
		}

		if err := d.handleValue(tok); err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) handleObject() error {
	frame := d.currentFrame()

	tok, err := d.jsonDecoder.ReadToken()
	if errors.Is(err, io.EOF) {
		return errors.New("unexpected end of JSON input")
	}
	if err != nil {
		return fmt.Errorf("read object token: %w", err)
	}

	if tok.Kind() == jsontext.EndObject.Kind() {
		d.popFrame()
		return nil
	}

	if err := d.prepareObjectField(frame, tok.String()); err != nil {
		return err
	}

	nextKind := d.jsonDecoder.PeekKind()
	handled, err := d.handleCompositeSpecialType(nextKind)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	valueTok, err := d.jsonDecoder.ReadToken()
	if errors.Is(err, io.EOF) {
		return errors.New("unexpected end of JSON input")
	}
	if err != nil {
		return fmt.Errorf("read field value token: %w", err)
	}

	return d.handleValue(valueTok)
}

func (d *Decoder) handleArray() error {
	frame := d.currentFrame()

	nextKind := d.jsonDecoder.PeekKind()
	if nextKind == 0 {
		return fmt.Errorf("peek array value: unexpected token at byte offset %d", d.jsonDecoder.InputOffset())
	}

	if nextKind == jsontext.EndArray.Kind() {
		_, err := d.jsonDecoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return errors.New("unexpected end of JSON input")
		}
		if err != nil {
			return fmt.Errorf("read array end: %w", err)
		}

		d.popFrame()
		return nil
	}

	if err := d.prepareArrayElement(frame); err != nil {
		return err
	}

	handled, err := d.handleCompositeSpecialType(nextKind)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	tok, err := d.jsonDecoder.ReadToken()
	if errors.Is(err, io.EOF) {
		return errors.New("unexpected end of JSON input")
	}
	if err != nil {
		return fmt.Errorf("read array token: %w", err)
	}

	return d.handleValue(tok)
}

func (d *Decoder) handleValue(tok jsontext.Token) error {
	frame := d.currentFrame()

	switch tok.Kind() {
	case jsontext.Null.Kind():
		for _, target := range frame.targets {
			if target.CanSet() {
				target.Set(reflect.Zero(target.Type()))
			}
		}
		d.popFrame()
		return nil

	case kindString, jsontext.True.Kind(), jsontext.False.Kind(), kindNumber:
		for _, target := range frame.targets {
			if !target.IsValid() {
				continue
			}

			valueTarget, assigned := ensureValue(target)
			if !assigned {
				continue
			}

			var unmarshaler graphql.Unmarshaler
			implements := false
			if valueTarget.CanAddr() {
				unmarshaler, implements = valueTarget.Addr().Interface().(graphql.Unmarshaler)
			} else if valueTarget.CanInterface() {
				unmarshaler, implements = valueTarget.Interface().(graphql.Unmarshaler)
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
				if err := unmarshalValue(tok, valueTarget); err != nil {
					return fmt.Errorf("unmarshal value: %w", err)
				}
			}
		}

		d.popFrame()
		return nil

	case jsontext.BeginObject.Kind():
		for i, target := range frame.targets {
			if !target.IsValid() {
				continue
			}
			valueTarget, assigned := ensureValue(target)
			if !assigned {
				frame.targets[i] = reflect.Value{}
				continue
			}
			frame.targets[i] = valueTarget
		}
		d.replaceCurrentKind(tok.Kind())
		return nil

	case jsontext.BeginArray.Kind():
		for i, target := range frame.targets {
			if !target.IsValid() {
				continue
			}
			valueTarget, assigned := ensureValue(target)
			if !assigned {
				frame.targets[i] = reflect.Value{}
				continue
			}
			if valueTarget.Kind() == reflect.Slice {
				valueTarget.Set(reflect.MakeSlice(valueTarget.Type(), 0, 0))
			}
			frame.targets[i] = valueTarget
		}
		d.replaceCurrentKind(tok.Kind())
		return nil

	case jsontext.EndObject.Kind(), jsontext.EndArray.Kind():
		d.popFrame()
		return nil

	default:
		return fmt.Errorf("unexpected token in JSON input (at byte offset %d)", d.jsonDecoder.InputOffset())
	}

	return nil
}

func (d *Decoder) prepareObjectField(frame *frame, key string) error {
	var fieldTargets []reflect.Value

	for _, target := range frame.targets {
		collectFieldTargets(target, key, &fieldTargets)
	}

	if len(fieldTargets) == 0 {
		return fmt.Errorf("struct field for %q doesn't exist in any of %v places to unmarshal (at byte offset %d)", key, len(frame.targets), d.jsonDecoder.InputOffset())
	}

	d.pushFrame(0, fieldTargets)
	return nil
}

func (d *Decoder) prepareArrayElement(frame *frame) error {
	someSliceExist := false
	var elements []reflect.Value

	for _, base := range frame.targets {
		target, ok := ensureValue(base)
		if !ok {
			elements = append(elements, reflect.Value{})
			continue
		}

		var elem reflect.Value
		if target.Kind() == reflect.Slice {
			target.Set(reflect.Append(target, reflect.Zero(target.Type().Elem())))
			elem = target.Index(target.Len() - 1)
			someSliceExist = true
		}

		elements = append(elements, elem)
	}

	if !someSliceExist {
		return fmt.Errorf("slice doesn't exist in any of %v places to unmarshal (at byte offset %d)", len(frame.targets), d.jsonDecoder.InputOffset())
	}

	d.pushFrame(0, elements)
	return nil
}

func collectFieldTargets(v reflect.Value, name string, dst *[]reflect.Value) {
	candidate := derefValue(v)
	if !candidate.IsValid() || candidate.Kind() != reflect.Struct {
		return
	}

	if field := fieldByGraphQLName(candidate, name); field.IsValid() {
		*dst = append(*dst, field)
	}

	for i := 0; i < candidate.NumField(); i++ {
		field := candidate.Type().Field(i)
		if isGraphQLFragment(field) || field.Anonymous {
			collectFieldTargets(candidate.Field(i), name, dst)
		}
	}
}

func (d *Decoder) handleCompositeSpecialType(peek jsontext.Kind) (bool, error) {
	if peek != jsontext.BeginObject.Kind() && peek != jsontext.BeginArray.Kind() {
		return false, nil
	}

	frame := d.currentFrame()
	hasSpecialType := false
	for _, target := range frame.targets {
		candidate := derefValue(target)
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

	for _, v := range frame.targets {
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

	d.popFrame()
	return true, nil
}

func (d *Decoder) state() jsontext.Kind {
	if len(d.frames) == 0 {
		return 0
	}

	return d.frames[len(d.frames)-1].kind
}

func (d *Decoder) currentFrame() *frame {
	return &d.frames[len(d.frames)-1]
}

func (d *Decoder) pushFrame(kind jsontext.Kind, targets []reflect.Value) {
	d.frames = append(d.frames, frame{kind: kind, targets: targets})
}

func (d *Decoder) popFrame() {
	if len(d.frames) == 0 {
		return
	}
	d.frames = d.frames[:len(d.frames)-1]
}

func (d *Decoder) replaceCurrentKind(kind jsontext.Kind) {
	d.frames[len(d.frames)-1].kind = kind
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
