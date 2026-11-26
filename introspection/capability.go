package introspection

// CapabilityQueryDocument asks a server which introspection fields and
// arguments it has. Servers built before the October 2021 GraphQL
// specification lack some of them and reject a query that uses them, so the
// introspection query must only use the ones reported here.
const CapabilityQueryDocument = `query Capabilities {
      typeType: __type(name: "__Type") {
        ...CapabilityType
      }
      fieldType: __type(name: "__Field") {
        ...CapabilityType
      }
      directiveType: __type(name: "__Directive") {
        ...CapabilityType
      }
      inputValueType: __type(name: "__InputValue") {
        ...CapabilityType
      }
    }

    fragment CapabilityType on __Type {
      fields {
        name
        args {
          name
        }
      }
    }`

// includeDeprecatedArgumentName is the argument that makes a server return
// deprecated members along with the others.
const includeDeprecatedArgumentName = "includeDeprecated"

// Capabilities are the optional parts of introspection a server supports.
// The zero value describes a server that supports none of them.
type Capabilities struct {
	// FieldArgumentDeprecation reports that __Field.args takes includeDeprecated.
	FieldArgumentDeprecation bool
	// InputFieldDeprecation reports that __Type.inputFields takes includeDeprecated.
	InputFieldDeprecation bool
	// DirectiveArgumentDeprecation reports that __Directive.args takes includeDeprecated.
	DirectiveArgumentDeprecation bool
	// InputValueDeprecation reports that __InputValue has isDeprecated and deprecationReason.
	InputValueDeprecation bool
	// RepeatableDirective reports that __Directive has isRepeatable.
	RepeatableDirective bool
}

// CapabilityQuery is the response to CapabilityQueryDocument.
type CapabilityQuery struct {
	TypeType       *CapabilityType `graphql:"typeType: __type(name: \"__Type\")"`
	FieldType      *CapabilityType `graphql:"fieldType: __type(name: \"__Field\")"`
	DirectiveType  *CapabilityType `graphql:"directiveType: __type(name: \"__Directive\")"`
	InputValueType *CapabilityType `graphql:"inputValueType: __type(name: \"__InputValue\")"`
}

// CapabilityType lists the fields of one introspection type.
type CapabilityType struct {
	Fields []*CapabilityField
}

// CapabilityField is a field of an introspection type with the names of its arguments.
type CapabilityField struct {
	Name string
	Args []*CapabilityArgument
}

// CapabilityArgument is an argument of a field of an introspection type.
type CapabilityArgument struct {
	Name string
}

// Capabilities derives what the server supports from the response.
//
// Arguments:
//   - none
//
// Returns:
//   - Capabilities: one flag per optional part, true when the server declares it
//
// Preconditions:
//   - none; a nil type or a nil field in the response counts as not declared
//
// Postconditions:
//   - InputValueDeprecation is true only when the server declares both
//     isDeprecated and deprecationReason on __InputValue
func (q CapabilityQuery) Capabilities() Capabilities {
	return Capabilities{
		FieldArgumentDeprecation:     q.FieldType.hasArgument("args", includeDeprecatedArgumentName),
		InputFieldDeprecation:        q.TypeType.hasArgument("inputFields", includeDeprecatedArgumentName),
		DirectiveArgumentDeprecation: q.DirectiveType.hasArgument("args", includeDeprecatedArgumentName),
		InputValueDeprecation:        q.InputValueType.field("isDeprecated") != nil && q.InputValueType.field("deprecationReason") != nil,
		RepeatableDirective:          q.DirectiveType.field("isRepeatable") != nil,
	}
}

// field finds a field of the introspection type by name.
//
// Arguments:
//   - fieldName: the name of the field to find
//
// Returns:
//   - *CapabilityField: the field, or nil when the type does not declare it
//
// Preconditions:
//   - none; t may be nil
//
// Postconditions:
//   - a non-nil result has the name fieldName
func (t *CapabilityType) field(fieldName string) *CapabilityField {
	if t == nil {
		return nil
	}

	for _, capabilityField := range t.Fields {
		if capabilityField != nil && capabilityField.Name == fieldName {
			return capabilityField
		}
	}

	return nil
}

// hasArgument reports whether a field of the introspection type takes an argument.
//
// Arguments:
//   - fieldName: the name of the field
//   - argumentName: the name of the argument
//
// Returns:
//   - bool: true when the type declares the field and the field takes the argument
//
// Preconditions:
//   - none; t may be nil
//
// Postconditions:
//   - none
func (t *CapabilityType) hasArgument(fieldName, argumentName string) bool {
	capabilityField := t.field(fieldName)
	if capabilityField == nil {
		return false
	}

	for _, capabilityArgument := range capabilityField.Args {
		if capabilityArgument != nil && capabilityArgument.Name == argumentName {
			return true
		}
	}

	return false
}
