package graphqljson

import (
	"bytes"
	"encoding"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/99designs/gqlgen/graphql"
)

var (
	jsonTextValueType    = reflect.TypeOf(jsontext.Value{})
	jsonRawMessageType   = reflect.TypeOf(stdjson.RawMessage{})
	jsonTextValuePtrType = reflect.TypeOf((*jsontext.Value)(nil))
	jsonRawPtrType       = reflect.TypeOf((*stdjson.RawMessage)(nil))

	kindBeginObject = jsontext.BeginObject.Kind()
	kindEndObject   = jsontext.EndObject.Kind()
	kindBeginArray  = jsontext.BeginArray.Kind()
	kindEndArray    = jsontext.EndArray.Kind()
	kindString      = jsontext.String("").Kind()
	kindNumber      = jsontext.Int(0).Kind()
	kindNull        = jsontext.Null.Kind()
	kindTrue        = jsontext.True.Kind()
	kindFalse       = jsontext.False.Kind()
)

// UnmarshalData parses the GraphQL response payload contained in data and stores
// the result into v, which must be a non-nil pointer.
func UnmarshalData(data jsontext.Value, v any) error {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("decode graphql data: decode json: cannot decode into non-pointer %T", v)
	}

	dec := jsontext.NewDecoder(bytes.NewReader(data))
	value, err := dec.ReadValue()
	if err != nil {
		return fmt.Errorf("decode graphql data: decode json: %w", err)
	}

	if err := decodeGraphQLValue(value, rv.Elem(), nil); err != nil {
		return fmt.Errorf("decode graphql data: decode json: %w", err)
	}

	if tok, err := dec.ReadToken(); err == nil {
		return fmt.Errorf("invalid token '%s' after top-level value (at byte offset %d)", tokenString(tok), dec.InputOffset())
	} else if err != io.EOF {
		return fmt.Errorf("decode graphql data: decode json: %w", err)
	}

	return nil
}

func tokenString(tok jsontext.Token) string {
	switch kind := tok.Kind(); kind {
	case kindBeginObject, kindEndObject, kindBeginArray, kindEndArray:
		return string(kind)
	case kindNull, kindTrue, kindFalse, kindString, kindNumber:
		return tok.String()
	default:
		return tok.String()
	}
}

func decodeGraphQLValue(data jsontext.Value, rv reflect.Value, used map[string]bool) error {
	if !rv.IsValid() {
		return nil
	}

	if rv.Kind() == reflect.Pointer {
		if isJSONNull(data) {
			rv.SetZero()
			return nil
		}
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return decodeGraphQLValue(data, rv.Elem(), used)
	}

	if handled, err := resolveCustomHandlers(data, rv); handled {
		return err
	}

	if isJSONNull(data) {
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

	switch {
	case rv.Type() == jsonTextValueType:
		rv.SetBytes(cloneBytes(data))
		return nil
	case rv.Type() == jsonRawMessageType:
		rv.SetBytes(cloneBytes(data))
		return nil
	case rv.CanAddr() && rv.Addr().Type() == jsonTextValuePtrType:
		rv.Addr().Elem().SetBytes(cloneBytes(data))
		return nil
	case rv.CanAddr() && rv.Addr().Type() == jsonRawPtrType:
		rv.Addr().Elem().SetBytes(cloneBytes(data))
		return nil
	}

	switch rv.Kind() {
	case reflect.Struct:
		return decodeStruct(data, rv, used, false)
	case reflect.Slice:
		return decodeSlice(data, rv)
	case reflect.Array:
		return decodeArray(data, rv)
	case reflect.Map:
		return decodeMap(data, rv)
	case reflect.Interface:
		var anyValue any
		var target any = &anyValue
		if err := json.Unmarshal(data, target); err != nil {
			return err
		}
		rv.Set(reflect.ValueOf(anyValue))
		return nil
	default:
		return json.Unmarshal(data, rv.Addr().Interface())
	}
}

func resolveCustomHandlers(data jsontext.Value, rv reflect.Value) (bool, error) {
	var target reflect.Value
	switch {
	case rv.CanAddr():
		target = rv.Addr()
	case rv.Kind() == reflect.Pointer && !rv.IsNil():
		target = rv
	}

	if target.IsValid() {
		if unmarshaler, ok := target.Interface().(graphql.Unmarshaler); ok {
			var payload any
			var target2 any = &payload
			if err := json.Unmarshal(data, target2); err != nil {
				return true, err
			}

			if err := unmarshaler.UnmarshalGQL(payload); err != nil {
				return true, err
			}

			if rv.CanSet() && target.Kind() == reflect.Pointer && target.Elem().IsValid() && rv.Kind() != reflect.Pointer {
				rv.Set(target.Elem())
			}

			return true, nil
		}
	}

	return resolveStandardInterfaces(data, rv)
}

func resolveStandardInterfaces(data jsontext.Value, rv reflect.Value) (bool, error) {
	if rv.CanAddr() {
		if u, ok := rv.Addr().Interface().(json.Unmarshaler); ok {
			return true, u.UnmarshalJSON(cloneBytes(data))
		}
		if t, ok := rv.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if isJSONNull(data) {
				rv.Set(reflect.Zero(rv.Type()))
				return true, nil
			}
			var s string
			var target any = &s
			if err := json.Unmarshal(data, target); err != nil {
				return true, err
			}
			return true, t.UnmarshalText([]byte(s))
		}
	}

	if rv.CanInterface() {
		if u, ok := rv.Interface().(json.Unmarshaler); ok {
			return true, u.UnmarshalJSON(cloneBytes(data))
		}
		if t, ok := rv.Interface().(encoding.TextUnmarshaler); ok {
			if isJSONNull(data) {
				rv.Set(reflect.Zero(rv.Type()))
				return true, nil
			}
			var s string
			var target any = &s
			if err := json.Unmarshal(data, target); err != nil {
				return true, err
			}
			return true, t.UnmarshalText([]byte(s))
		}
	}

	return false, nil
}

func decodeSlice(data jsontext.Value, rv reflect.Value) error {
	if isJSONNull(data) {
		rv.SetZero()
		return nil
	}

	var rawElems []jsontext.Value
	if err := json.Unmarshal(data, &rawElems); err != nil {
		return err
	}

	slice := reflect.MakeSlice(rv.Type(), len(rawElems), len(rawElems))
	for i, raw := range rawElems {
		if err := decodeGraphQLValue(raw, slice.Index(i), nil); err != nil {
			return err
		}
	}

	rv.Set(slice)
	return nil
}

func decodeArray(data jsontext.Value, rv reflect.Value) error {
	if isJSONNull(data) {
		for i := 0; i < rv.Len(); i++ {
			rv.Index(i).Set(reflect.Zero(rv.Index(i).Type()))
		}
		return nil
	}

	var rawElems []jsontext.Value
	if err := json.Unmarshal(data, &rawElems); err != nil {
		return err
	}

	for i := 0; i < rv.Len(); i++ {
		if i < len(rawElems) {
			if err := decodeGraphQLValue(rawElems[i], rv.Index(i), nil); err != nil {
				return err
			}
		} else {
			rv.Index(i).Set(reflect.Zero(rv.Index(i).Type()))
		}
	}
	return nil
}

func decodeMap(data jsontext.Value, rv reflect.Value) error {
	if isJSONNull(data) {
		rv.SetZero()
		return nil
	}

	tmp := reflect.New(rv.Type()).Interface()
	if err := json.Unmarshal(data, tmp); err != nil {
		return err
	}

	rv.Set(reflect.ValueOf(tmp).Elem())
	return nil
}

type objectField struct {
	value  jsontext.Value
	offset int64
}

// IsUnion checks if a struct type represents a GraphQL union type.
// A union type is identified by having 2 or more named fields that are
// pointers to anonymous structs (excluding anonymous embedded fields).
func IsUnion(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}

	unionFieldCount := 0
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}
		// Skip anonymous embedded fields (these are fragments)
		if field.Anonymous {
			continue
		}
		// Check if this is a named field pointing to an anonymous struct
		if field.Type.Kind() == reflect.Pointer &&
			field.Type.Elem().Kind() == reflect.Struct &&
			field.Type.Elem().Name() == "" {
			unionFieldCount++
		}
	}

	return unionFieldCount >= 2
}

func decodeStruct(data jsontext.Value, rv reflect.Value, used map[string]bool, ignoreUnknown bool) error {
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %s", rv.Kind())
	}

	fields, err := parseJSONObject(data)
	if err != nil {
		return err
	}

	if used == nil {
		used = make(map[string]bool, len(fields))
	}

	info := buildStructInfo(rv.Type())

	// Check if __typename exists in the data
	typenameField, hasTypename := fields["__typename"]
	var typename string

	if hasTypename {
		var target any = &typename
		if err := json.Unmarshal(typenameField.value, target); err != nil {
			return fmt.Errorf("decode __typename: %w", err)
		}
		used["__typename"] = true
	}

	// Check if this struct is a union type and collect union fields
	var unionFields []struct {
		info structFieldInfo
	}
	isUnionType := IsUnion(rv.Type())
	if isUnionType {
		var unionFieldsPresentInData int
		for _, fi := range info.fields {
			// Skip anonymous embedded fields (these are fragments, not union alternatives)
			if fi.anonymous {
				continue
			}
			field := rv.Field(fi.index)
			fieldType := rv.Type().Field(fi.index)
			// Check if this is a named field pointing to an anonymous struct
			if field.Kind() == reflect.Pointer &&
				field.Type().Elem().Kind() == reflect.Struct &&
				fieldType.Type.Elem().Name() == "" { // anonymous struct
				unionFields = append(unionFields, struct{ info structFieldInfo }{info: fi})

				// Check if this field is present and non-null in the data
				key := fi.jsonName
				if key == "" {
					key = fi.name
				}
				if dataField, exists := fields[key]; exists && !isJSONNull(dataField.value) {
					unionFieldsPresentInData++
				}
			}
		}

		// If we have 2+ union fields present in data but no __typename, it's ambiguous
		if unionFieldsPresentInData > 1 && !hasTypename {
			return errors.New("__typename is required for union decoding")
		}
	}

	fallbackAssigned := false

	// First pass: regular fields.
	for _, fi := range info.fields {
		if fi.omit || fi.anonymous {
			continue
		}

		// Skip inline fragments (named fields with no JSON tag and anonymous struct type)
		// Inline fragments spread their fields into the parent, so they don't have a JSON key
		fieldValue := rv.Field(fi.index)
		fieldType := rv.Type().Field(fi.index)
		if fi.jsonName == "" && fieldValue.Kind() == reflect.Struct && fieldType.Type.Name() == "" {
			continue
		}

		key, ok := locateFieldKey(fi, fields)
		if !ok {
			if fi.jsonUnknown && fi.jsonName == "" {
				fieldValue.Set(reflect.Zero(fieldValue.Type()))
			}
			continue
		}

		used[key] = true
		raw := fields[key].value
		if fi.jsonUnknown && fi.jsonName != "" {
			if err := decodeGraphQLValue(raw, fieldValue, nil); err != nil {
				return err
			}
			if info.fallbackIndex >= 0 && fi.index == info.fallbackIndex {
				fallbackAssigned = true
			}
			continue
		}
		if err := decodeGraphQLValue(raw, fieldValue, nil); err != nil {
			return err
		}
		if info.fallbackIndex >= 0 && fi.index == info.fallbackIndex && fi.jsonUnknown && fi.jsonName == "" {
			fallbackAssigned = true
		}
	}

	// Second pass: inline fragments and anonymous fields.
	for _, fi := range info.fields {
		fieldValue := rv.Field(fi.index)
		fieldType := rv.Type().Field(fi.index)

		// Check if this is an inline fragment (named field with no JSON tag and anonymous struct type)
		// or an anonymous embedded field
		isInlineFragment := !fi.anonymous && fi.jsonName == "" && fieldValue.Kind() == reflect.Struct && fieldType.Type.Name() == ""
		if !(isInlineFragment || fi.anonymous) {
			continue
		}

		if fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				elem := reflect.New(fieldValue.Type().Elem())
				fieldValue.Set(elem)
			}
			fieldValue = fieldValue.Elem()
		}
		if !fieldValue.IsValid() {
			continue
		}
		if fieldValue.Kind() != reflect.Struct {
			continue
		}
		if err := decodeStruct(data, fieldValue, used, true); err != nil {
			return err
		}
	}

	// Only process as union if this is a union type
	if typename != "" && isUnionType {
		remaining := make(map[string]objectField)
		for key, field := range fields {
			if used[key] {
				continue
			}
			remaining[key] = field
		}

		matched := false
		for _, entry := range unionFields {
			fieldValue := rv.Field(entry.info.index)
			name := entry.info.name
			if entry.info.jsonName != "" {
				name = entry.info.jsonName
			}
			if strings.EqualFold(name, typename) || strings.EqualFold(entry.info.name, typename) {
				matched = true

				if fieldValue.Kind() == reflect.Pointer {
					if fieldValue.IsNil() {
						fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
					}
					target := fieldValue.Elem()

					unionInfo := buildStructInfo(fieldValue.Type().Elem())
					var unionEntries []objectFieldEntry
					unionUsed := map[string]bool{}
					for _, ufi := range unionInfo.fields {
						candidate := ufi.jsonName
						if candidate == "" {
							candidate = ufi.name
						}
						actualKey := matchKeyInsensitive(remaining, candidate)
						if actualKey == "" {
							continue
						}

						field := remaining[actualKey]
						unionEntries = append(unionEntries, objectFieldEntry{name: actualKey, value: field.value, offset: field.offset})
						unionUsed[actualKey] = true
					}

					if len(unionEntries) > 0 {
						payload := buildUnknownValue(unionEntries)
						if err := decodeGraphQLValue(payload, target, nil); err != nil {
							return err
						}
						for key := range unionUsed {
							used[key] = true
						}
					}
				} else {
					fieldValue.Set(reflect.Zero(fieldValue.Type()))
				}
				continue
			}
			fieldValue.Set(reflect.Zero(fieldValue.Type()))
		}
		if !matched {
			return fmt.Errorf("union type %q not mapped", typename)
		}
	}

	leftovers := collectLeftovers(fields, used)

	if info.fallbackIndex >= 0 {
		fbField := rv.Field(info.fallbackIndex)
		if !fallbackAssigned {
			if err := setUnknownFallback(fbField, leftovers); err != nil {
				return err
			}
		}
		leftovers = nil
	}

	if len(leftovers) > 0 {
		if ignoreUnknown {
			return nil
		}
		entry := leftovers[0]
		return &unknownFieldError{name: entry.name, offset: entry.offset, places: 1}
	}

	return nil
}

func locateFieldKey(fi structFieldInfo, fields map[string]objectField) (string, bool) {
	// Check json tag or field name
	for key := range fields {
		if fi.jsonName != "" {
			if key == fi.jsonName {
				return key, true
			}
			continue
		}
		if strings.EqualFold(key, fi.name) {
			return key, true
		}
	}
	return "", false
}

func collectLeftovers(fields map[string]objectField, used map[string]bool) []objectFieldEntry {
	var leftovers []objectFieldEntry
	for key, field := range fields {
		if used[key] {
			continue
		}
		leftovers = append(leftovers, objectFieldEntry{name: key, value: field.value, offset: field.offset})
	}
	return leftovers
}

func matchKeyInsensitive(fields map[string]objectField, target string) string {
	for key := range fields {
		if strings.EqualFold(key, target) {
			return key
		}
	}
	return ""
}

type objectFieldEntry struct {
	name   string
	value  jsontext.Value
	offset int64
}

func setUnknownFallback(field reflect.Value, leftovers []objectFieldEntry) error {
	if len(leftovers) == 0 {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	aggregated := buildUnknownValue(leftovers)
	return decodeGraphQLValue(aggregated, field, nil)
}

func buildUnknownValue(entries []objectFieldEntry) jsontext.Value {
	if len(entries) == 1 {
		return cloneBytes(entries[0].value)
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, entry := range entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Quote(entry.name))
		buf.WriteByte(':')
		buf.Write(entry.value)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

type unknownFieldError struct {
	name   string
	offset int64
	places int
}

func (e *unknownFieldError) Error() string {
	return fmt.Sprintf("struct field for %q doesn't exist in any of %d places to unmarshal (at byte offset %d)", e.name, e.places, e.offset)
}

func parseJSONObject(data jsontext.Value) (map[string]objectField, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]objectField{}, nil
	}

	dec := jsontext.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	if tok.Kind() != kindBeginObject {
		return nil, errors.New("expected JSON object")
	}

	fields := make(map[string]objectField)
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if tok.Kind() == kindEndObject {
			break
		}
		if tok.Kind() != kindString {
			return nil, errors.New("unexpected non-string key in JSON object")
		}
		key := tok.String()
		offset := dec.InputOffset()
		val, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		fields[key] = objectField{value: cloneBytes(val), offset: offset}
	}

	return fields, nil
}

func isJSONNull(data jsontext.Value) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func cloneBytes(data jsontext.Value) []byte {
	if data == nil {
		return nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out
}
