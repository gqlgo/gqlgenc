package clientv2

import (
	"bytes"
	"context"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"

	"github.com/99designs/gqlgen/graphql"
)

// Encoder encodes GraphQL request bodies and variables to JSON.
//
// It delegates to encoding/json/v2 configured with encoding/json v1 semantics
// (omitempty on Go emptiness, HTML escaping, sorted map keys, nil maps as null)
// and layers the gqlgen marshaler conventions on top: values implementing
// graphql.ContextMarshaler or graphql.Marshaler are encoded with MarshalGQLContext
// or MarshalGQL, which take precedence over json.Marshaler and encoding.TextMarshaler.
type Encoder struct {
	// NilSliceAsEmptyArray encodes nil slices as [] instead of null.
	// See Options.EncodeNilSliceAsEmptyArray for details.
	NilSliceAsEmptyArray bool
}

// MarshalJSON encodes v to JSON with a default Encoder.
//
// Arguments:
//   - ctx: passed to MarshalGQLContext of values implementing graphql.ContextMarshaler
//   - v: the value to encode; nil is encoded as null
//
// Returns:
//   - []byte: the JSON encoding of v
//   - error: non-nil if v or one of its elements cannot be encoded
//
// Preconditions:
//   - none
//
// Postconditions:
//   - nil slices are encoded as null; use a Client with Options.EncodeNilSliceAsEmptyArray to encode them as []
func MarshalJSON(ctx context.Context, v any) ([]byte, error) {
	encoder := &Encoder{}

	return encoder.encode(ctx, v)
}

// marshalJSON encodes a request body honoring the client's encoder options.
//
// Arguments:
//   - ctx: passed to MarshalGQLContext of values implementing graphql.ContextMarshaler
//   - v: the request body to encode
//
// Returns:
//   - []byte: the JSON encoding of v
//   - error: non-nil if v or one of its elements cannot be encoded
//
// Preconditions:
//   - none
//
// Postconditions:
//   - nil slices are encoded as [] when c.EncodeNilSliceAsEmptyArray is true, otherwise as null
func (c *Client) marshalJSON(ctx context.Context, v any) ([]byte, error) {
	encoder := &Encoder{NilSliceAsEmptyArray: c.EncodeNilSliceAsEmptyArray}

	return encoder.encode(ctx, v)
}

// Encode encodes a reflect.Value to JSON.
//
// Arguments:
//   - v: the value to encode; an invalid reflect.Value is encoded as null
//
// Returns:
//   - []byte: the JSON encoding of v
//   - error: non-nil if v or one of its elements cannot be encoded
//
// Preconditions:
//   - v must be obtained from an exported value so that v.Interface() does not panic
//
// Postconditions:
//   - values implementing graphql.ContextMarshaler receive context.Background(); use MarshalJSON to pass a context
func (e *Encoder) Encode(v reflect.Value) ([]byte, error) {
	if !v.IsValid() {
		return []byte("null"), nil
	}

	return e.encode(context.Background(), v.Interface())
}

// encode is the shared implementation of MarshalJSON and Encode.
//
// Arguments:
//   - ctx: passed to MarshalGQLContext of values implementing graphql.ContextMarshaler
//   - v: the value to encode; nil is encoded as null
//
// Returns:
//   - []byte: the JSON encoding of v
//   - error: non-nil if v or one of its elements cannot be encoded, including invalid JSON written by MarshalGQL
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the output follows encoding/json v1 semantics except for the gqlgen marshaler hooks and e.NilSliceAsEmptyArray
func (e *Encoder) encode(ctx context.Context, v any) ([]byte, error) {
	return jsonv2.Marshal(v, jsonv2.JoinOptions(
		jsonv1.DefaultOptionsV1(),
		jsonv2.WithMarshalers(gqlMarshalers(ctx, e.NilSliceAsEmptyArray)),
		jsonv2.FormatNilSliceAsNull(!e.NilSliceAsEmptyArray),
	))
}

// gqlMarshalers builds the type-specific marshalers for the gqlgen conventions.
//
// Arguments:
//   - ctx: passed to MarshalGQLContext
//   - nilSliceAsEmptyArray: when true, a nil slice implementing either interface is encoded with its method instead of as null
//
// Returns:
//   - *jsonv2.Marshalers: marshalers for graphql.ContextMarshaler and graphql.Marshaler, in that order of precedence
//
// Preconditions:
//   - none
//
// Postconditions:
//   - nil values implementing either interface (nil pointers, and nil slices, maps and interfaces reached through the
//     pointer that encoding/json/v2 passes for addressable values) are encoded as null without calling the method
//   - the bytes written by MarshalGQLContext or MarshalGQL are validated as a single JSON value
func gqlMarshalers(ctx context.Context, nilSliceAsEmptyArray bool) *jsonv2.Marshalers {
	return jsonv2.JoinMarshalers(
		jsonv2.MarshalToFunc(func(enc *jsontext.Encoder, m graphql.ContextMarshaler) error {
			if isNilMarshaler(reflect.ValueOf(m), nilSliceAsEmptyArray) {
				return enc.WriteToken(jsontext.Null)
			}

			var buf bytes.Buffer

			err := m.MarshalGQLContext(ctx, &buf)
			if err != nil {
				return err
			}

			return enc.WriteValue(buf.Bytes())
		}),
		jsonv2.MarshalToFunc(func(enc *jsontext.Encoder, m graphql.Marshaler) error {
			if isNilMarshaler(reflect.ValueOf(m), nilSliceAsEmptyArray) {
				return enc.WriteToken(jsontext.Null)
			}

			var buf bytes.Buffer
			m.MarshalGQL(&buf)

			return enc.WriteValue(buf.Bytes())
		}),
	)
}

// isNilMarshaler reports whether the receiver passed to a gqlgen marshaler hook holds a nil value that must be
// encoded as null instead of calling its marshal method.
//
// encoding/json/v2 hands addressable values to interface marshalers as *T even when T itself implements the
// interface, so a nil named slice such as `type IDs []string` arrives as a non-nil *IDs. Looking through that
// pointer keeps every nil value encoded as null, as the reflect-based encoder before v0.40.0 did.
//
// Arguments:
//   - v: the marshaler receiver as passed by encoding/json/v2
//   - nilSliceAsEmptyArray: when true, a nil slice is reported as non-nil so that its marshal method produces the array
//
// Returns:
//   - bool: true if v is a nil pointer, or a pointer to (or a value of) a nil map, interface, chan, func, or slice
//
// Preconditions:
//   - none
//
// Postconditions:
//   - a nil slice is reported as nil only when nilSliceAsEmptyArray is false
//   - values of other kinds, including invalid values, are reported as non-nil
func isNilMarshaler(v reflect.Value, nilSliceAsEmptyArray bool) bool {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}

		v = v.Elem()
	}

	if v.Kind() == reflect.Slice && nilSliceAsEmptyArray {
		return false
	}

	return isNil(v)
}

// isNil reports whether v holds a nil value of a nillable kind.
//
// Arguments:
//   - v: the value to inspect
//
// Returns:
//   - bool: true if v is a nil pointer, map, slice, chan, func, or interface
//
// Preconditions:
//   - none
//
// Postconditions:
//   - values of other kinds, including invalid values, are reported as non-nil
func isNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
