package introspection

// includeDeprecatedArgument は、deprecated なメンバーも返すようサーバーに求める引数。
const includeDeprecatedArgument = "(" + includeDeprecatedArgumentName + ": true)"

// NewQuery は、capabilities を持つサーバーへ送る introspection クエリを組み立てる。
// オペレーション名は Query になる。
//
// 事前条件: サーバーは、capabilities で true になっている部分のすべてに対応している。
// 対応していない部分を使うと、サーバーはクエリを拒否する。
// 事後条件: 任意のフィールドと引数は、対応する capability が true のときだけクエリに含まれる。
// NewQuery(Capabilities{}) は Introspection と等しい。
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

// optional は、サーバーがその部分に対応しているときだけ text を返す。対応していなければ "" を返す。
func optional(supported bool, text string) string {
	if supported {
		return text
	}

	return ""
}
