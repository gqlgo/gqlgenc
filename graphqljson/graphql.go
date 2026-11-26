/*
MIT License

Copyright (c) 2017 Dmitri Shuralyov

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package graphqljson provides a function for decoding JSON
// into a GraphQL query data structure.
package graphqljson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

// Reference: https://blog.gopheracademy.com/advent-2017/custom-json-unmarshaler-for-graphql-client/

// UnmarshalData parses the JSON-encoded GraphQL response data and stores
// the result in the GraphQL query data structure pointed to by v.
//
// The implementation is created on top of the JSON tokenizer available
// in "encoding/json".Decoder.
func UnmarshalData(data json.RawMessage, v any) error {
	d := newDecoder(bytes.NewBuffer(data))

	err := d.Decode(v)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	tok, err := d.jsonDecoder.Token()
	if errors.Is(err, io.EOF) {
		// Expect to get io.EOF. There shouldn't be any more
		// tokens left after we've decoded v successfully.
		return nil
	}

	if err != nil {
		return fmt.Errorf("invalid input after top-level value: %w", err)
	}

	return fmt.Errorf("invalid token '%v' after top-level value", tok)
}

// Decoder is a JSON Decoder that performs custom unmarshaling behavior
// for GraphQL query data structures. It's implemented on top of a JSON tokenizer.
type Decoder struct {
	jsonDecoder *json.Decoder

	// Stack of the JSON objects and arrays we're in the middle of, innermost last.
	containers []container

	// Stacks of values where to unmarshal.
	// The top of each stack is the reflect.Value where to unmarshal next JSON value.
	//
	// The reason there's more than one stack is because we might be unmarshaling
	// a single JSON value into multiple GraphQL fragments or embedded structs, so
	// we keep track of them all.
	stacks []valueStack
}

// containerKind is the kind of JSON value the decoder is in the middle of.
type containerKind int

const (
	// containerKindNone means the decoder is at the top level, outside any object or array.
	containerKindNone containerKind = iota
	// containerKindObject means the next token is a key or the end of the object.
	containerKindObject
	// containerKindArray means the next token is an element or the end of the array.
	containerKindArray
)

// container is one JSON object or array the decoder is in the middle of.
type container struct {
	kind containerKind

	// typename is the __typename read so far among the keys of an object. It
	// discriminates the inline fragments found in that object. It is meaningful
	// only when isTypenameRead is true.
	typename string

	// isTypenameRead is false until a string __typename has been read: keys that
	// come before __typename, and objects without one, are decoded without it.
	isTypenameRead bool
}

// fragmentMatch is how the __typename of the object being decoded relates to
// the inline fragment a stack was created from. It decides what the fragment
// receives from the keys of that object:
//
//	match         | nil pointer allocated | fields stored    | reset on '}'
//	Undetermined  | yes                   | yes              | no
//	Applies       | yes                   | yes              | no
//	HeldBack      | no                    | if non-nil       | no
//	RuledOut      | no                    | no               | yes
//
// An object starts Undetermined and moves once to one of the other three when
// its __typename is read.
type fragmentMatch int

const (
	// fragmentMatchUndetermined means no __typename has been read for the object yet.
	fragmentMatchUndetermined fragmentMatch = iota
	// fragmentMatchApplies means the stack is not an inline fragment, or the
	// inline fragment applies to the __typename.
	fragmentMatchApplies
	// fragmentMatchHeldBack means the inline fragment does not apply to the
	// __typename but cannot be ruled out: it has no possibleTypes tag (generated
	// by an older version or written by hand), or the stack is below the object
	// the fragment was found in.
	fragmentMatchHeldBack
	// fragmentMatchRuledOut means the inline fragment has a possibleTypes tag,
	// the stack is at the object the fragment was found in, and the __typename
	// is not one of the possible types.
	fragmentMatchRuledOut
)

// allocatesNilPointer reports whether a nil pointer on top of a stack with this
// match may be allocated to receive a field.
//
// Arguments:
//   - none
//
// Returns:
//   - bool: true for Undetermined, which falls back to the field-presence check, and Applies
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (m fragmentMatch) allocatesNilPointer() bool {
	return m == fragmentMatchUndetermined || m == fragmentMatchApplies
}

// valueStack is one stack of values to unmarshal into.
type valueStack struct {
	values []reflect.Value

	// fragType is "TypeName" for stacks created from "... on TypeName" inline
	// fragments, otherwise "".
	fragType string

	// possibleTypes are the object types that satisfy the type condition of the inline
	// fragment, from its gqlgenc:"possibleTypes=..." tag. nil when the tag is absent.
	possibleTypes []string
}

// appliesTo reports whether the inline fragment the stack was created from applies to
// an object whose __typename is typename.
//
// Arguments:
//   - typename: the __typename of the object being decoded
//
// Returns:
//   - bool: true for stacks not created from a typed inline fragment; otherwise whether
//     typename is one of the possible types, or equals the type condition when the
//     fragment has no possibleTypes tag
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the stack is unchanged
func (s valueStack) appliesTo(typename string) bool {
	switch {
	case s.possibleTypes != nil:
		return slices.Contains(s.possibleTypes, typename)
	case s.fragType != "":
		return s.fragType == typename
	default:
		return true
	}
}

// top returns the value on top of the stack, where the next JSON value goes.
//
// Arguments:
//   - none
//
// Returns:
//   - reflect.Value: the last pushed value
//
// Preconditions:
//   - the stack is not empty
//
// Postconditions:
//   - the stack is unchanged
func (s valueStack) top() reflect.Value {
	return s.values[len(s.values)-1]
}

func newDecoder(r io.Reader) *Decoder {
	jsonDecoder := json.NewDecoder(r)
	jsonDecoder.UseNumber()

	return &Decoder{jsonDecoder: jsonDecoder}
}

// Decode decodes a single JSON value from d.tokenizer into v.
func (d *Decoder) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer {
		return fmt.Errorf("cannot decode into non-pointer %T", v)
	}

	d.stacks = []valueStack{{values: []reflect.Value{rv.Elem()}}}

	err := d.decode()
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	return nil
}

// decode decodes a single JSON value from d.tokenizer into d.stacks.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil if the input ends early, is not valid JSON, or does not fit the target types
//
// Preconditions:
//   - d.stacks holds one stack per target value
//
// Postconditions:
//   - every stack has been consumed once the top-level value is decoded
func (d *Decoder) decode() error {
	// The loop invariant is that the top of each stack
	// is where we try to unmarshal the next JSON value we see.
	for len(d.stacks) > 0 {
		tok, err := d.readToken()
		if err != nil {
			return err
		}

		switch {
		// Are we inside an object and seeing next key (rather than end of object)?
		case d.containerKind() == containerKindObject && tok != json.Delim('}'):
			key, ok := tok.(string)
			if !ok {
				return errors.New("unexpected non-key in JSON input")
			}

			tok, err = d.decodeObjectKey(key)
			if err != nil {
				return err
			}

		// Are we inside an array and seeing next value (rather than end of array)?
		case d.containerKind() == containerKindArray && tok != json.Delim(']'):
			err = d.pushArrayElement()
			if err != nil {
				return err
			}
		}

		switch tok := tok.(type) {
		case nil: // Handle null values correctly.
			d.assignNull()
		case string, json.Number, bool, json.RawMessage, map[string]any:
			err = d.assignScalar(tok)
			if err != nil {
				return err
			}
		case json.Delim:
			err = d.handleDelim(tok)
			if err != nil {
				return err
			}
		default:
			return errors.New("unexpected token in JSON input")
		}
	}

	return nil
}

// readToken reads the next JSON token.
//
// Arguments:
//   - none
//
// Returns:
//   - json.Token: the token read
//   - error: non-nil if the input ends or is malformed
//
// Preconditions:
//   - none
//
// Postconditions:
//   - io.EOF is reported as an unexpected end of input
func (d *Decoder) readToken() (json.Token, error) {
	tok, err := d.jsonDecoder.Token()
	if err != nil {
		return nil, wrapReadError(err)
	}

	return tok, nil
}

// wrapReadError converts a tokenizer error into a decoding error.
//
// Arguments:
//   - err: the error returned by the JSON decoder, or nil
//
// Returns:
//   - error: nil for nil, an "unexpected end of JSON input" error for io.EOF, otherwise err wrapped
//
// Preconditions:
//   - none
//
// Postconditions:
//   - errors other than io.EOF can be unwrapped to err
func wrapReadError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		return errors.New("unexpected end of JSON input")
	}

	return fmt.Errorf(": %w", err)
}

// decodeObjectKey pushes the struct field matching key onto every stack and
// reads the token of the key's value.
//
// Arguments:
//   - key: the object key just read
//
// Returns:
//   - json.Token: the value of the key; a whole value for json.RawMessage and map[string]any
//     fields, and for a key only inline fragments ruled out by __typename have
//   - error: non-nil if no stack has a matching field or the value cannot be read
//
// Preconditions:
//   - d.containerKind() is containerKindObject
//
// Postconditions:
//   - every stack has one more entry: the matching field, or an invalid value
//   - a __typename value is recorded for the current object depth before the fields are pushed
func (d *Decoder) decodeObjectKey(key string) (json.Token, error) {
	// If this key is __typename, eagerly read its value so we can use it
	// to discriminate which inline fragment pointers to initialize below.
	// This must happen before the nil-pointer init loop.
	// A null __typename yields a nil token, so track the read with a flag.
	var (
		earlyReadTok json.Token
		earlyRead    bool
	)

	if key == "__typename" {
		tok, err := d.readToken()
		if err != nil {
			return nil, err
		}

		earlyReadTok, earlyRead = tok, true

		if s, ok := tok.(string); ok {
			d.recordTypename(s)
		}
	}

	// Checked before pushField, which replaces the top of every stack.
	ruledOutFieldExists := d.ruledOutFragmentHasField(key)

	matchingFieldValue := d.pushField(key)
	if matchingFieldValue == nil && !ruledOutFieldExists {
		return nil, fmt.Errorf("struct field for %q doesn't exist in any of %v places to unmarshal", key, len(d.stacks))
	}

	if earlyRead {
		return earlyReadTok, nil
	}

	if matchingFieldValue == nil {
		// Only inline fragments that __typename rules out have the field, e.g. an interface
		// the client schema does not know the type implements. Every stack has an invalid
		// value on top, so the whole value is read and dropped.
		return d.readRawValue()
	}

	return d.readValue(*matchingFieldValue)
}

// pushField pushes the field named key of the struct on top of every stack.
//
// Arguments:
//   - key: the GraphQL name of the field
//
// Returns:
//   - *reflect.Value: the last stack's matching field, or nil if no stack has one
//
// Preconditions:
//   - none
//
// Postconditions:
//   - stacks without a matching field, and inline fragments ruled out by __typename,
//     get an invalid value pushed instead
//   - a nil pointer to a struct that has the field is allocated first, unless a
//     __typename rules out the inline fragment
func (d *Decoder) pushField(key string) *reflect.Value {
	// The last matching one is the one considered
	var matchingFieldValue *reflect.Value

	for i := range d.stacks {
		v := d.stacks[i].top()

		match := d.fragmentMatchOf(i)

		if match == fragmentMatchRuledOut {
			d.stacks[i].values = append(d.stacks[i].values, reflect.Value{})

			continue
		}

		// If v is a nil pointer, check whether the key exists in the pointed-to
		// type before initializing — preserves nil for non-matching union variants.
		// When a __typename was seen, also require the inline fragment to apply to it.
		if v.Kind() == reflect.Pointer && v.IsNil() && v.CanSet() && hasGraphQLField(v.Type(), key) && match.allocatesNilPointer() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		var f reflect.Value
		if v.Kind() == reflect.Struct {
			f = fieldByGraphQLName(v, key)
			if f.IsValid() {
				matchingFieldValue = &f
			}
		}

		d.stacks[i].values = append(d.stacks[i].values, f)
	}

	return matchingFieldValue
}

// ruledOutFragmentHasField reports whether an inline fragment that __typename
// rules out has the field named key.
//
// Arguments:
//   - key: the GraphQL name of the field
//
// Returns:
//   - bool: true if the struct on top of at least one ruled-out stack has the field
//
// Preconditions:
//   - the top of every stack is the struct being decoded, i.e. pushField has not
//     been called for key yet
//
// Postconditions:
//   - the stacks are unchanged
func (d *Decoder) ruledOutFragmentHasField(key string) bool {
	for i := range d.stacks {
		if d.fragmentMatchOf(i) == fragmentMatchRuledOut && hasGraphQLField(d.stacks[i].top().Type(), key) {
			return true
		}
	}

	return false
}

// readValue reads the JSON value for field.
//
// Arguments:
//   - field: the field the value will be stored in
//
// Returns:
//   - json.Token: the whole value for json.RawMessage and map[string]any fields, otherwise the next token
//   - error: non-nil if the input ends or is malformed
//
// Preconditions:
//   - the tokenizer is positioned at the value
//
// Postconditions:
//   - none
func (d *Decoder) readValue(field reflect.Value) (json.Token, error) {
	switch field.Type() {
	case reflect.TypeFor[json.RawMessage]():
		return d.readRawValue()
	case reflect.TypeFor[map[string]any]():
		var data map[string]any

		err := d.jsonDecoder.Decode(&data)
		if err != nil {
			return nil, wrapReadError(err)
		}

		return data, nil
	default:
		return d.readToken()
	}
}

// readRawValue reads a whole JSON value without decoding it.
//
// Arguments:
//   - none
//
// Returns:
//   - json.Token: the value as a json.RawMessage
//   - error: non-nil if the input ends or is malformed
//
// Preconditions:
//   - the tokenizer is positioned at the value
//
// Postconditions:
//   - the tokenizer is positioned after the value
func (d *Decoder) readRawValue() (json.Token, error) {
	var data json.RawMessage

	err := d.jsonDecoder.Decode(&data)
	if err != nil {
		return nil, wrapReadError(err)
	}

	return data, nil
}

// pushArrayElement appends a zero element to the slice on top of every stack and pushes it.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil if no stack has a slice on top
//
// Preconditions:
//   - d.containerKind() is containerKindArray
//
// Postconditions:
//   - stacks without a slice on top get an invalid value pushed instead
func (d *Decoder) pushArrayElement() error {
	someSliceExist := false

	for i := range d.stacks {
		v := d.stacks[i].top()
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		var f reflect.Value

		if v.Kind() == reflect.Slice {
			v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem()))) // v = append(v, T).
			f = v.Index(v.Len() - 1)
			someSliceExist = true
		}

		d.stacks[i].values = append(d.stacks[i].values, f)
	}

	if !someSliceExist {
		return fmt.Errorf("slice doesn't exist in any of %v places to unmarshal", len(d.stacks))
	}

	return nil
}

// assignNull stores a JSON null: the value on top of every stack is reset and popped.
//
// Arguments:
//   - none
//
// Returns:
//   - none
//
// Preconditions:
//   - none
//
// Postconditions:
//   - settable values on top of the stacks are set to their zero value; nil for pointers and slices
//   - every stack is popped
func (d *Decoder) assignNull() {
	for i := range d.stacks {
		v := d.stacks[i].top()
		if !v.CanSet() {
			// If v is not settable, skip the operation to prevent panicking.
			continue
		}

		// Pointers and slices become nil; other kinds keep their zero value.
		v.Set(reflect.Zero(v.Type()))
	}

	d.popAllVs()
}

// assignScalar stores a scalar JSON value into the value on top of every stack and pops it.
//
// Arguments:
//   - tok: a string, json.Number, bool, json.RawMessage, or map[string]any
//
// Returns:
//   - error: non-nil if a value rejects tok
//
// Preconditions:
//   - none
//
// Postconditions:
//   - nil pointers on top of the stacks are allocated before assignment
//   - values implementing graphql.Unmarshaler receive tok through UnmarshalGQL
//   - every stack is popped
func (d *Decoder) assignScalar(tok json.Token) error {
	for i := range d.stacks {
		v := d.stacks[i].top()
		if !v.IsValid() {
			continue
		}

		// Initialize the pointer if it is nil
		if v.Kind() == reflect.Pointer && v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		// Handle both pointer and non-pointer types
		target := v
		if v.Kind() == reflect.Pointer {
			target = v.Elem()
		}

		// Check if the type of target (or its address) implements graphql.Unmarshaler
		var (
			unmarshaler graphql.Unmarshaler
			ok          bool
		)

		if target.CanAddr() {
			unmarshaler, ok = target.Addr().Interface().(graphql.Unmarshaler)
		} else if target.CanInterface() {
			unmarshaler, ok = target.Interface().(graphql.Unmarshaler)
		}

		if ok {
			err := unmarshaler.UnmarshalGQL(tok)
			if err != nil {
				return fmt.Errorf("unmarshal gql error: %w", err)
			}
		} else {
			// Use the standard unmarshal method for non-custom types
			err := unmarshalValue(tok, target)
			if err != nil {
				return fmt.Errorf(": %w", err)
			}
		}
	}

	d.popAllVs()

	return nil
}

// handleDelim opens or closes an object or array.
//
// Arguments:
//   - tok: the delimiter just read
//
// Returns:
//   - error: non-nil for a delimiter other than {, }, [, or ]
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the containers and the stacks reflect the opened or closed value
//   - on '}', inline fragments of the object ruled out by its __typename are reset
func (d *Decoder) handleDelim(tok json.Delim) error {
	switch tok {
	case '{':
		d.openObject()
	case '[':
		d.openArray()
	case '}', ']':
		// End of object or array.
		if tok == '}' {
			d.resetRuledOutFragments()
		}

		d.popAllVs()
		d.popContainer()
	default:
		return errors.New("unexpected delimiter in JSON input")
	}

	return nil
}

// openObject starts an object.
//
// Arguments:
//   - none
//
// Returns:
//   - none
//
// Preconditions:
//   - the tokenizer just returned '{'
//
// Postconditions:
//   - nil pointers on top of the stacks are allocated
//   - a stack is added for every GraphQL fragment or embedded struct reachable
//     from the values on top of the stacks
func (d *Decoder) openObject() {
	d.pushContainer(containerKindObject)

	frontier := make([]reflect.Value, len(d.stacks)) // Places to look for GraphQL fragments/embedded structs.
	for i := range d.stacks {
		v := d.stacks[i].top()
		frontier[i] = v
		// TODO: Do this recursively or not? Add a test case if needed.
		if v.Kind() == reflect.Pointer && v.IsNil() {
			v.Set(reflect.New(v.Type().Elem())) // v = new(T).
		}
	}
	// Find GraphQL fragments/embedded structs recursively, adding to frontier
	// as new ones are discovered and exploring them further.
	for len(frontier) > 0 {
		v := frontier[0]
		frontier = frontier[1:]

		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			continue
		}

		for i := range v.NumField() {
			field := v.Type().Field(i)
			if isGraphQLFragment(field) || field.Anonymous {
				// Add GraphQL fragment or embedded struct.
				d.stacks = append(d.stacks, valueStack{
					values:        []reflect.Value{v.Field(i)},
					fragType:      inlineFragmentType(field),
					possibleTypes: possibleTypesOf(field),
				})
				frontier = append(frontier, v.Field(i))
			}
		}
	}
}

// openArray starts an array.
//
// Arguments:
//   - none
//
// Returns:
//   - none
//
// Preconditions:
//   - the tokenizer just returned '['
//
// Postconditions:
//   - slices on top of the stacks are reset to empty
func (d *Decoder) openArray() {
	d.pushContainer(containerKindArray)

	for i := range d.stacks {
		v := d.stacks[i].top()
		// TODO: Confirm this is needed, write a test case.
		// if v.Kind() == reflect.Pointer && v.IsNil() {
		//	v.Set(reflect.New(v.Type().Elem())) // v = new(T).
		//}

		// Reset slice to empty (in case it had non-zero initial value).
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		if v.Kind() != reflect.Slice {
			continue
		}

		v.Set(reflect.MakeSlice(v.Type(), 0, 0)) // v = make(T, 0, 0).
	}
}

// pushContainer records that an object or array has been opened.
//
// Arguments:
//   - kind: containerKindObject or containerKindArray
//
// Returns:
//   - none
//
// Preconditions:
//   - the tokenizer just returned the opening delimiter of kind
//
// Postconditions:
//   - the new container is the innermost one, with no __typename read
func (d *Decoder) pushContainer(kind containerKind) {
	d.containers = append(d.containers, container{kind: kind})
}

// popContainer records that the innermost object or array has been closed.
//
// Arguments:
//   - none
//
// Returns:
//   - none
//
// Preconditions:
//   - d.containers is not empty
//
// Postconditions:
//   - the __typename read for a closed object is forgotten with it
func (d *Decoder) popContainer() {
	d.containers = d.containers[:len(d.containers)-1]
}

// containerKind reports the kind of the innermost container.
//
// Arguments:
//   - none
//
// Returns:
//   - containerKind: containerKindNone at the top level
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (d *Decoder) containerKind() containerKind {
	if len(d.containers) == 0 {
		return containerKindNone
	}

	return d.containers[len(d.containers)-1].kind
}

// recordTypename stores the __typename of the object being decoded.
//
// Arguments:
//   - typename: the string value of the __typename key
//
// Returns:
//   - none
//
// Preconditions:
//   - d.containerKind() is containerKindObject
//
// Postconditions:
//   - fragmentMatchOf no longer returns fragmentMatchUndetermined for this object
func (d *Decoder) recordTypename(typename string) {
	object := &d.containers[len(d.containers)-1]
	object.typename = typename
	object.isTypenameRead = true
}

// popAllVs pops from all stacks, keeping only non-empty ones.
func (d *Decoder) popAllVs() {
	var nonEmpty []valueStack

	for i := range d.stacks {
		d.stacks[i].values = d.stacks[i].values[:len(d.stacks[i].values)-1]
		if len(d.stacks[i].values) > 0 {
			nonEmpty = append(nonEmpty, d.stacks[i])
		}
	}

	d.stacks = nonEmpty
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

// hasGraphQLField reports whether a struct type has an exported field with a GraphQL name.
//
// Arguments:
//   - t: a struct type, or a pointer to one
//   - name: the GraphQL name of the field
//
// Returns:
//   - bool: whether the struct has the field; false when t is neither a struct nor a pointer to one
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func hasGraphQLField(t reflect.Type, name string) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return false
	}

	return fieldByGraphQLName(reflect.New(t).Elem(), name).IsValid()
}

// hasGraphQLName reports whether struct field f has GraphQL name.
func hasGraphQLName(f reflect.StructField, name string) bool {
	value, ok := f.Tag.Lookup("graphql")
	if !ok {
		// TODO: caseconv package is relatively slow. Optimize it, then consider using it here.
		// return caseconv.MixedCapsToLowerCamelCase(f.Name) == name
		return strings.EqualFold(f.Name, name)
	}

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
func unmarshalValue(value json.Token, v reflect.Value) error {
	b, err := json.Marshal(value) // TODO: Short-circuit (if profiling says it's worth it).
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	err = json.Unmarshal(b, v.Addr().Interface())
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	return nil
}

// fragmentMatchOf reports how the __typename of the object being decoded relates
// to the inline fragment a stack was created from.
//
// Only inline fragments with a gqlgenc:"possibleTypes=..." tag are ruled out. Fragments
// without it, generated by older versions or written by hand, keep receiving every field
// they have; only their nil pointers are held back.
//
// Arguments:
//   - i: the index of the stack
//
// Returns:
//   - fragmentMatch: Undetermined when no __typename has been read for the object;
//     Applies when the stack is not an inline fragment or the fragment applies to the
//     __typename; RuledOut when a fragment with a possibleTypes tag does not apply and the
//     stack is at the object the fragment was found in; HeldBack otherwise
//
// Preconditions:
//   - 0 <= i < len(d.stacks)
//   - d.containerKind() is containerKindObject
//
// Postconditions:
//   - none
func (d *Decoder) fragmentMatchOf(i int) fragmentMatch {
	object := d.containers[len(d.containers)-1]
	if !object.isTypenameRead {
		return fragmentMatchUndetermined
	}

	stack := d.stacks[i]
	if stack.appliesTo(object.typename) {
		return fragmentMatchApplies
	}

	// The stack holds only the fragment itself while the keys of the object the fragment
	// was found in are decoded; deeper entries are fields of the fragment.
	if stack.possibleTypes != nil && len(stack.values) == 1 {
		return fragmentMatchRuledOut
	}

	return fragmentMatchHeldBack
}

// resetRuledOutFragments resets the inline fragments that the __typename of the object
// being closed rules out, discarding the fields decoded before __typename was read.
//
// Arguments:
//   - none
//
// Returns:
//   - none
//
// Preconditions:
//   - the tokenizer just returned the '}' of the object; d.containers still includes it
//
// Postconditions:
//   - each ruled out fragment is set to its zero value: nil for a pointer
func (d *Decoder) resetRuledOutFragments() {
	for i := range d.stacks {
		if d.fragmentMatchOf(i) != fragmentMatchRuledOut {
			continue
		}

		fragment := d.stacks[i].top()
		if fragment.CanSet() {
			fragment.Set(reflect.Zero(fragment.Type()))
		}
	}
}

// inlineFragmentType returns the concrete type name from a "... on TypeName" graphql
// tag, or "" if the field is not a typed inline fragment.
func inlineFragmentType(f reflect.StructField) string {
	value, ok := f.Tag.Lookup("graphql")
	if !ok {
		return ""
	}

	value = strings.TrimSpace(value)

	const prefix = "... on "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

// possibleTypesOf returns the object types listed in the possibleTypes tag of an inline fragment.
//
// Arguments:
//   - f: a struct field, e.g. tagged `graphql:"... on Node" gqlgenc:"possibleTypes=Image,Video"`
//
// Returns:
//   - []string: the comma-separated type names after "possibleTypes=", or nil if f has no
//     gqlgenc tag starting with it
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func possibleTypesOf(f reflect.StructField) []string {
	value, ok := f.Tag.Lookup("gqlgenc")
	if !ok {
		return nil
	}

	typeNames, ok := strings.CutPrefix(strings.TrimSpace(value), "possibleTypes=")
	if !ok {
		return nil
	}

	return strings.Split(typeNames, ",")
}
