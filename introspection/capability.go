package introspection

import (
	"context"

	"github.com/Yamashou/gqlgenc/v3/client"
)

// CapabilityQueryDocument は、サーバーが introspection のどのフィールドと引数を持つかを問い合わせる。
// 2021年10月版の GraphQL 仕様より前に作られたサーバーはその一部を持たず、それを使うクエリを
// 拒否する。そのため introspection クエリは、ここで報告されたものだけを使う。
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

// includeDeprecatedArgumentName は、deprecated なメンバーも返すようサーバーに求める引数の名前。
const includeDeprecatedArgumentName = "includeDeprecated"

// Capabilities は、introspection の任意の部分のうちサーバーが対応しているものを表す。
// ゼロ値は、どれにも対応していないサーバーを表す。
type Capabilities struct {
	// FieldArgumentDeprecation は、__Field.args が includeDeprecated を取ることを表す。
	FieldArgumentDeprecation bool
	// InputFieldDeprecation は、__Type.inputFields が includeDeprecated を取ることを表す。
	InputFieldDeprecation bool
	// DirectiveArgumentDeprecation は、__Directive.args が includeDeprecated を取ることを表す。
	DirectiveArgumentDeprecation bool
	// InputValueDeprecation は、__InputValue が isDeprecated と deprecationReason を持つことを表す。
	InputValueDeprecation bool
	// RepeatableDirective は、__Directive が isRepeatable を持つことを表す。
	RepeatableDirective bool
}

// CapabilityQuery は CapabilityQueryDocument への応答。
type CapabilityQuery struct {
	TypeType       *CapabilityType `json:"typeType"`
	FieldType      *CapabilityType `json:"fieldType"`
	DirectiveType  *CapabilityType `json:"directiveType"`
	InputValueType *CapabilityType `json:"inputValueType"`
}

// CapabilityType は、introspection の1つの型が持つフィールドの一覧。
type CapabilityType struct {
	Fields []*CapabilityField `json:"fields"`
}

// CapabilityField は、introspection の型のフィールドと、その引数の名前。
type CapabilityField struct {
	Name string                `json:"name"`
	Args []*CapabilityArgument `json:"args"`
}

// CapabilityArgument は、introspection の型のフィールドの引数。
type CapabilityArgument struct {
	Name string `json:"name"`
}

// Capabilities は、応答からサーバーが対応している部分を求める。
//
// 事前条件: なし。応答の中の nil の型・nil のフィールドは、宣言されていないものとして扱う。
// 事後条件: InputValueDeprecation は、サーバーが __InputValue に isDeprecated と
// deprecationReason の両方を宣言しているときだけ true になる。
func (q CapabilityQuery) Capabilities() Capabilities {
	return Capabilities{
		FieldArgumentDeprecation:     q.FieldType.hasArgument("args", includeDeprecatedArgumentName),
		InputFieldDeprecation:        q.TypeType.hasArgument("inputFields", includeDeprecatedArgumentName),
		DirectiveArgumentDeprecation: q.DirectiveType.hasArgument("args", includeDeprecatedArgumentName),
		InputValueDeprecation:        q.InputValueType.field("isDeprecated") != nil && q.InputValueType.field("deprecationReason") != nil,
		RepeatableDirective:          q.DirectiveType.field("isRepeatable") != nil,
	}
}

// field は、introspection の型のフィールドを名前で探す。t が nil のとき、または型がそのフィールドを
// 宣言していないときは nil を返す。
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

// hasArgument は、introspection の型が fieldName のフィールドを宣言していて、そのフィールドが
// argumentName の引数を取るかを返す。t は nil でもよい。
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

// capabilityOperation は、capability の問い合わせの型付きオペレーション。変数は取らない。
var capabilityOperation = client.Operation[client.Query, struct{}, CapabilityQuery]{
	Name:     "Capabilities",
	Document: CapabilityQueryDocument,
}

// remoteCapabilities は、introspection の任意の部分のうちどれに対応しているかをサーバーに問い合わせる。
//
// 事前条件: gqlClient は nil でない。
// 事後条件: 問い合わせに失敗したとき (古いサーバーが問い合わせを拒否した・通信に失敗したなど) は、
// エラーを報告せずゼロ値を返す。続く introspection クエリは任意の部分を使わず、失敗が続くなら
// そのクエリがエラーを報告する。
func remoteCapabilities(ctx context.Context, gqlClient *client.Client) Capabilities {
	capabilityQuery, err := gqlClient.Post(ctx, capabilityOperation, struct{}{})
	if err != nil {
		return Capabilities{}
	}

	return capabilityQuery.Capabilities()
}
