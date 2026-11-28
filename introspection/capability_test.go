package introspection

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestCapabilityQueryDocument は、capability の問い合わせが introspection のスキーマに対して
// 妥当なクエリであることを確認する。妥当でないと、現行のサーバーも問い合わせを拒否し、任意の部分を
// 使えなくなる。
func TestCapabilityQueryDocument(t *testing.T) {
	t.Parallel()

	if diff := cmp.Diff("", introspectionQueryErrors(t, CapabilityQueryDocument)); diff != "" {
		t.Errorf("query validation errors diff(-want +got): %s", diff)
	}
}

func TestCapabilityQuery_Capabilities(t *testing.T) {
	t.Parallel()

	// includeDeprecated は、includeDeprecated だけを持つ引数の一覧を返す
	includeDeprecated := func() []*CapabilityArgument {
		return []*CapabilityArgument{{Name: "includeDeprecated"}}
	}

	type fields struct {
		capabilityQuery CapabilityQuery
	}

	type want struct {
		capabilities Capabilities
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			// 型を1つも返さない応答は、任意の部分のどれにも対応していないものとして扱う
			name:   "空の応答はどの capability も持たない",
			fields: fields{capabilityQuery: CapabilityQuery{}},
			want:   want{capabilities: Capabilities{}},
		},
		{
			// 2021年10月版の仕様より前のサーバー。fields の includeDeprecated だけを持つ
			name: "任意の部分ができる前のサーバーはどの capability も持たない",
			fields: fields{capabilityQuery: CapabilityQuery{
				TypeType:       &CapabilityType{Fields: []*CapabilityField{{Name: "fields", Args: includeDeprecated()}, {Name: "inputFields"}}},
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args"}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "name"}, {Name: "args"}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "name"}, {Name: "defaultValue"}}},
			}},
			want: want{capabilities: Capabilities{}},
		},
		{
			name: "現行のサーバーはすべての capability を持つ",
			fields: fields{capabilityQuery: CapabilityQuery{
				TypeType:       &CapabilityType{Fields: []*CapabilityField{{Name: "inputFields", Args: includeDeprecated()}}},
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args", Args: includeDeprecated()}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "isRepeatable"}, {Name: "args", Args: includeDeprecated()}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "isDeprecated"}, {Name: "deprecationReason"}}},
			}},
			want: want{capabilities: Capabilities{
				FieldArgumentDeprecation:     true,
				InputFieldDeprecation:        true,
				DirectiveArgumentDeprecation: true,
				InputValueDeprecation:        true,
				RepeatableDirective:          true,
			}},
		},
		{
			// __Directive は isRepeatable を持つが args は includeDeprecated を取らない。
			// __InputValue は isDeprecated だけを持ち deprecationReason を持たないので、対応していない扱いになる
			name: "capability は部分ごとに判定する",
			fields: fields{capabilityQuery: CapabilityQuery{
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args", Args: includeDeprecated()}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "isRepeatable"}, {Name: "args"}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "isDeprecated"}}},
			}},
			want: want{capabilities: Capabilities{
				FieldArgumentDeprecation: true,
				RepeatableDirective:      true,
			}},
		},
		{
			name: "nil のフィールドと引数は読み飛ばす",
			fields: fields{capabilityQuery: CapabilityQuery{
				FieldType: &CapabilityType{Fields: []*CapabilityField{nil, {Name: "args", Args: []*CapabilityArgument{nil, {Name: "includeDeprecated"}}}}},
			}},
			want: want{capabilities: Capabilities{FieldArgumentDeprecation: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.fields.capabilityQuery.Capabilities()

			if diff := cmp.Diff(tt.want.capabilities, got); diff != "" {
				t.Errorf("capabilities diff(-want +got): %s", diff)
			}
		})
	}
}
