package introspection

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

type TypeKind string

const (
	TypeKindScalar      TypeKind = "SCALAR"
	TypeKindObject      TypeKind = "OBJECT"
	TypeKindInterface   TypeKind = "INTERFACE"
	TypeKindUnion       TypeKind = "UNION"
	TypeKindEnum        TypeKind = "ENUM"
	TypeKindInputObject TypeKind = "INPUT_OBJECT"
	TypeKindList        TypeKind = "LIST"
	TypeKindNonNull     TypeKind = "NON_NULL"
)

type FullTypes []*FullType

func (fs FullTypes) NameMap() map[string]*FullType {
	typeMap := make(map[string]*FullType)
	for _, typ := range fs {
		typeMap[*typ.Name] = typ
	}

	return typeMap
}

type FullType struct {
	Kind          TypeKind      `json:"kind"`
	Name          *string       `json:"name"`
	Description   *string       `json:"description"`
	Fields        []*FieldValue `json:"fields"`
	InputFields   []*InputValue `json:"inputFields"`
	Interfaces    []*TypeRef    `json:"interfaces"`
	EnumValues    []*EnumValue  `json:"enumValues"`
	PossibleTypes []*TypeRef    `json:"possibleTypes"`
}

type EnumValue struct {
	Description       *string `json:"description"`
	DeprecationReason *string `json:"deprecationReason"`
	Name              string  `json:"name"`
	IsDeprecated      bool    `json:"isDeprecated"`
}

type FieldValue struct {
	Type              TypeRef       `json:"type"`
	Description       *string       `json:"description"`
	DeprecationReason *string       `json:"deprecationReason"`
	Name              string        `json:"name"`
	Args              []*InputValue `json:"args"`
	IsDeprecated      bool          `json:"isDeprecated"`
}

type InputValue struct {
	Type         TypeRef `json:"type"`
	Description  *string `json:"description"`
	DefaultValue *string `json:"defaultValue"`
	Name         string  `json:"name"`
}

type TypeRef struct {
	Name   *string  `json:"name"`
	OfType *TypeRef `json:"ofType"`
	Kind   TypeKind `json:"kind"`
}

type Query struct {
	Schema struct {
		QueryType struct {
			Name *string `json:"name"`
		} `json:"queryType"`
		MutationType *struct {
			Name *string `json:"name"`
		} `json:"mutationType"`
		SubscriptionType *struct {
			Name *string `json:"name"`
		} `json:"subscriptionType"`
		Types      FullTypes        `json:"types"`
		Directives []*DirectiveType `json:"directives"`
	} `json:"__schema"`
}

type DirectiveType struct {
	Name        string        `json:"name"`
	Description *string       `json:"description"`
	Locations   []string      `json:"locations"`
	Args        []*InputValue `json:"args"`
}
