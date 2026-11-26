package introspection

// Introspection is the introspection query for a server that supports none of
// the optional parts of introspection. It equals NewQuery(Capabilities{}).
const Introspection = `query Query {
      __schema {
        queryType { name }
        mutationType { name }
        subscriptionType { name }
        types {
          ...FullType
        }
        directives {
          name
          description
          locations
          args {
            ...InputValue
          }
        }
      }
    }

    fragment FullType on __Type {
      kind
      name
      description
      fields(includeDeprecated: true) {
        name
        description
        args {
          ...InputValue
        }
        type {
          ...TypeRef
        }
        isDeprecated
        deprecationReason
      }
      inputFields {
        ...InputValue
      }
      interfaces {
        ...TypeRef
      }
      enumValues(includeDeprecated: true) {
        name
        description
        isDeprecated
        deprecationReason
      }
      possibleTypes {
        ...TypeRef
      }
    }

    fragment InputValue on __InputValue {
      name
      description
      type { ...TypeRef }
      defaultValue
    }

    fragment TypeRef on __Type {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                  ofType {
                    kind
                    name
                  }
                }
              }
            }
          }
        }
      }
    }`

// includeDeprecatedArgument makes a server return deprecated members along with the others.
const includeDeprecatedArgument = "(" + includeDeprecatedArgumentName + ": true)"

// NewQuery builds the introspection query for a server with the given capabilities.
//
// Arguments:
//   - capabilities: the optional parts of introspection the server supports
//
// Returns:
//   - string: the introspection query document, with the operation named Query
//
// Preconditions:
//   - the server supports every part set in capabilities; otherwise it rejects the query
//
// Postconditions:
//   - the query uses an optional field or argument only when its capability is set
//   - NewQuery(Capabilities{}) equals Introspection
func NewQuery(capabilities Capabilities) string {
	return `query Query {
      __schema {
        queryType { name }
        mutationType { name }
        subscriptionType { name }
        types {
          ...FullType
        }
        directives {
          name
          description` + optional(capabilities.RepeatableDirective, `
          isRepeatable`) + `
          locations
          args` + optional(capabilities.DirectiveArgumentDeprecation, includeDeprecatedArgument) + ` {
            ...InputValue
          }
        }
      }
    }

    fragment FullType on __Type {
      kind
      name
      description
      fields(includeDeprecated: true) {
        name
        description
        args` + optional(capabilities.FieldArgumentDeprecation, includeDeprecatedArgument) + ` {
          ...InputValue
        }
        type {
          ...TypeRef
        }
        isDeprecated
        deprecationReason
      }
      inputFields` + optional(capabilities.InputFieldDeprecation, includeDeprecatedArgument) + ` {
        ...InputValue
      }
      interfaces {
        ...TypeRef
      }
      enumValues(includeDeprecated: true) {
        name
        description
        isDeprecated
        deprecationReason
      }
      possibleTypes {
        ...TypeRef
      }
    }

    fragment InputValue on __InputValue {
      name
      description
      type { ...TypeRef }
      defaultValue` + optional(capabilities.InputValueDeprecation, `
      isDeprecated
      deprecationReason`) + `
    }

    fragment TypeRef on __Type {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                  ofType {
                    kind
                    name
                  }
                }
              }
            }
          }
        }
      }
    }`
}

// optional returns text when the server supports the part it belongs to.
//
// Arguments:
//   - supported: whether the server supports the part
//   - text: the part of the query
//
// Returns:
//   - string: text when supported is true, otherwise ""
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func optional(supported bool, text string) string {
	if supported {
		return text
	}

	return ""
}
