package introspection

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSchemaFromIntrospection_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "no mutation in schema",
			filename: "testdata/introspection_result_no_mutation.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parseIntrospectionQuery() panicked: %v", r)
				}
			}()

			query := readQueryResult(t, tt.filename)

			ast := SchemaFromIntrospection("test", query)
			if ast == nil {
				t.Error("SchemaFromIntrospection() returned nil")
			}
		})
	}
}

func readQueryResult(t *testing.T, filename string) Query {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Error reading file %s: %v", filename, err)
	}

	query := Query{}

	err = json.Unmarshal(data, &query)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON: %v", err)
	}

	return query
}
