package introspection

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapabilityQuery_Capabilities(t *testing.T) {
	t.Parallel()

	includeDeprecated := []*CapabilityArgument{{Name: "includeDeprecated"}}

	tests := []struct {
		name            string
		capabilityQuery CapabilityQuery
		expected        Capabilities
	}{
		{
			name:            "an empty response supports nothing",
			capabilityQuery: CapabilityQuery{},
			expected:        Capabilities{},
		},
		{
			name: "a server that predates the optional parts supports nothing",
			capabilityQuery: CapabilityQuery{
				TypeType:       &CapabilityType{Fields: []*CapabilityField{{Name: "fields", Args: includeDeprecated}, {Name: "inputFields"}}},
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args"}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "name"}, {Name: "args"}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "name"}, {Name: "defaultValue"}}},
			},
			expected: Capabilities{},
		},
		{
			name: "a current server supports everything",
			capabilityQuery: CapabilityQuery{
				TypeType:       &CapabilityType{Fields: []*CapabilityField{{Name: "inputFields", Args: includeDeprecated}}},
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args", Args: includeDeprecated}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "isRepeatable"}, {Name: "args", Args: includeDeprecated}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "isDeprecated"}, {Name: "deprecationReason"}}},
			},
			expected: Capabilities{
				FieldArgumentDeprecation:     true,
				InputFieldDeprecation:        true,
				DirectiveArgumentDeprecation: true,
				InputValueDeprecation:        true,
				RepeatableDirective:          true,
			},
		},
		{
			name: "each part is detected on its own",
			capabilityQuery: CapabilityQuery{
				FieldType:      &CapabilityType{Fields: []*CapabilityField{{Name: "args", Args: includeDeprecated}}},
				DirectiveType:  &CapabilityType{Fields: []*CapabilityField{{Name: "isRepeatable"}, {Name: "args"}}},
				InputValueType: &CapabilityType{Fields: []*CapabilityField{{Name: "isDeprecated"}}},
			},
			expected: Capabilities{
				FieldArgumentDeprecation: true,
				RepeatableDirective:      true,
			},
		},
		{
			name: "nil fields and arguments are skipped",
			capabilityQuery: CapabilityQuery{
				FieldType: &CapabilityType{Fields: []*CapabilityField{nil, {Name: "args", Args: []*CapabilityArgument{nil, {Name: "includeDeprecated"}}}}},
			},
			expected: Capabilities{FieldArgumentDeprecation: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, test.capabilityQuery.Capabilities())
		})
	}
}
