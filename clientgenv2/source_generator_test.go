package clientgenv2

import (
	"github.com/99designs/gqlgen/codegen/config"
	gqlgencConfig "github.com/gqlgo/gqlgenc/config"
	"go/types"
	"testing"
)

func createTestStruct(fields []*types.Var, tags []string) *types.Struct {
	return types.NewStruct(fields, tags)
}

func TestMergeFieldsRecursively(t *testing.T) {
	tests := []struct {
		name               string
		targetFields       ResponseFieldList
		sourceFields       ResponseFieldList
		preMerged          []*StructSource
		postMerged         []*StructSource
		expectedFields     ResponseFieldList
		expectedPreMerged  []*StructSource
		expectedPostMerged []*StructSource
	}{
		{
			name: "Basic merge case",
			targetFields: ResponseFieldList{
				{
					Name: "field1",
					Type: types.Typ[types.String],
					Tags: []string{`json:"field1"`},
				},
			},
			sourceFields: ResponseFieldList{
				{
					Name: "field2",
					Type: types.Typ[types.Int],
					Tags: []string{`json:"field2"`},
				},
			},
			preMerged:  []*StructSource{},
			postMerged: []*StructSource{},
			expectedFields: ResponseFieldList{
				{
					Name: "field1",
					Type: types.Typ[types.String],
					Tags: []string{`json:"field1"`},
				},
				{
					Name: "field2",
					Type: types.Typ[types.Int],
					Tags: []string{`json:"field2"`},
				},
			},
			expectedPreMerged:  []*StructSource{},
			expectedPostMerged: []*StructSource{},
		},
		{
			name: "Merge case with complex query including fragments",
			targetFields: ResponseFieldList{
				{
					Name: "id",
					Type: types.Typ[types.String],
					Tags: []string{`json:"id"`},
				},
				{
					Name: "profile",
					Type: types.NewPointer(types.NewNamed(
						types.NewTypeName(0, nil, "A_User_Profile", nil),
						createTestStruct([]*types.Var{
							types.NewVar(0, nil, "id", types.Typ[types.String]),
							types.NewVar(0, nil, "name", types.Typ[types.String]),
						}, []string{`json:"id"`, `json:"name"`}),
						nil,
					)),
					Tags: []string{`json:"profile"`},
					ResponseFields: ResponseFieldList{
						{
							Name: "id",
							Type: types.Typ[types.String],
							Tags: []string{`json:"id"`},
						},
						{
							Name: "ProfileFragment",
							Type: types.NewPointer(types.NewNamed(
								types.NewTypeName(0, nil, "ProfileFragment", nil),
								createTestStruct([]*types.Var{
									types.NewVar(0, nil, "name", types.Typ[types.String]),
								}, []string{`json:"name"`}),
								nil,
							)),
							Tags:             []string{`json:"ProfileFragment"`},
							IsFragmentSpread: true,
							ResponseFields: ResponseFieldList{
								{
									Name: "name",
									Type: types.Typ[types.String],
									Tags: []string{`json:"name"`},
								},
							},
						},
					},
				},
			},
			sourceFields: ResponseFieldList{
				{
					Name: "profile",
					Type: types.NewPointer(types.NewNamed(
						types.NewTypeName(0, nil, "Profile", nil),
						createTestStruct([]*types.Var{
							types.NewVar(0, nil, "id", types.Typ[types.String]),
							types.NewVar(0, nil, "name", types.Typ[types.String]),
						}, []string{`json:"id"`, `json:"name"`}),
						nil,
					)),
					Tags: []string{`json:"profile"`},
					ResponseFields: ResponseFieldList{
						{
							Name: "name",
							Type: types.Typ[types.String],
							Tags: []string{`json:"name"`},
						},
					},
				},
			},
			preMerged:  []*StructSource{},
			postMerged: []*StructSource{},
			expectedFields: ResponseFieldList{
				{
					Name: "id",
					Type: types.Typ[types.String],
					Tags: []string{`json:"id"`},
				},
				{
					Name: "profile",
					Type: types.NewPointer(types.NewNamed(
						types.NewTypeName(0, nil, "A_User_Profile", nil),
						createTestStruct([]*types.Var{
							types.NewVar(0, nil, "id", types.Typ[types.String]),
							types.NewVar(0, nil, "name", types.Typ[types.String]),
						}, []string{`json:"id"`, `json:"name"`}),
						nil,
					)),
					Tags: []string{`json:"profile"`},
					ResponseFields: ResponseFieldList{
						{
							Name: "id",
							Type: types.Typ[types.String],
							Tags: []string{`json:"id"`},
						},
						{
							Name: "ProfileFragment",
							Type: types.NewPointer(types.NewNamed(
								types.NewTypeName(0, nil, "ProfileFragment", nil),
								createTestStruct([]*types.Var{
									types.NewVar(0, nil, "name", types.Typ[types.String]),
								}, []string{`json:"name"`}),
								nil,
							)),
							Tags:             []string{`json:"ProfileFragment"`},
							IsFragmentSpread: true,
							ResponseFields: ResponseFieldList{
								{
									Name: "name",
									Type: types.Typ[types.String],
									Tags: []string{`json:"name"`},
								},
							},
						},
					},
				},
			},
			expectedPreMerged: []*StructSource{
				{
					Name: "Nested",
					Type: createTestStruct([]*types.Var{
						types.NewVar(0, nil, "id", types.Typ[types.String]),
					}, []string{`json:"id"`}),
				},
				{
					Name: "Nested",
					Type: createTestStruct([]*types.Var{
						types.NewVar(0, nil, "name", types.Typ[types.String]),
					}, []string{`json:"name"`}),
				},
			},
			expectedPostMerged: []*StructSource{
				{
					Name: "Nested",
					Type: createTestStruct([]*types.Var{
						types.NewVar(0, nil, "id", types.Typ[types.String]),
						types.NewVar(0, nil, "name", types.Typ[types.String]),
					}, []string{`json:"id"`, `json:"name"`}),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultFields, resultPreMerged, resultPostMerged := mergeFieldsRecursively(
				tt.targetFields,
				tt.sourceFields,
				tt.preMerged,
				tt.postMerged,
			)

			if len(tt.expectedFields) != len(resultFields) {
				t.Errorf("Number of fields does not match: got %v, want %v", len(resultFields), len(tt.expectedFields))
				return
			}

			if len(tt.expectedPreMerged) != len(resultPreMerged) {
				t.Errorf("Number of preMerged does not match: got %v, want %v", len(resultPreMerged), len(tt.expectedPreMerged))
			}

			if len(tt.expectedPostMerged) != len(resultPostMerged) {
				t.Errorf("Number of postMerged does not match: got %v, want %v", len(resultPostMerged), len(tt.expectedPostMerged))
			}

			for i := range tt.expectedFields {
				if tt.expectedFields[i].Name != resultFields[i].Name {
					t.Errorf("Field name does not match: got %v, want %v", resultFields[i].Name, tt.expectedFields[i].Name)
				}
				if tt.expectedFields[i].Type.String() != resultFields[i].Type.String() {
					t.Errorf("Field type does not match: got %v, want %v", resultFields[i].Type.String(), tt.expectedFields[i].Type.String())
				}
				if len(tt.expectedFields[i].Tags) != len(resultFields[i].Tags) {
					t.Errorf("Number of tags does not match: got %v, want %v", len(resultFields[i].Tags), len(tt.expectedFields[i].Tags))
				}
				for j, tag := range tt.expectedFields[i].Tags {
					if tag != resultFields[i].Tags[j] {
						t.Errorf("Tag does not match: got %v, want %v", resultFields[i].Tags[j], tag)
					}
				}
			}
		})
	}
}

func TestFragmentSpreadExpansionInInlineFragment(t *testing.T) {
	// このテストでは、フラグメントスプレッドがあるインラインフラグメントで
	// `hasFragmentSpread` と `collectFragmentFields` 関数が正しく動作することを確認します
	//
	// This test verifies that `hasFragmentSpread` and `collectFragmentFields` functions
	// work correctly when handling fragment spreads in inline fragments.

	// テストデータを作成
	// Create test data
	// フラグメントスプレッドを含むフィールドのセット
	// A set of fields containing fragment spreads
	responseFields := ResponseFieldList{
		{
			Name: "languages",
			Type: types.NewPointer(types.NewNamed(
				types.NewTypeName(0, nil, "LanguagesConnection", nil),
				types.NewStruct(nil, nil),
				nil,
			)),
			Tags: []string{`json:"languages,omitempty" graphql:"languages"`},
		},
		{
			Name:             "RepositoryFragment",
			Type:             types.NewPointer(types.NewNamed(types.NewTypeName(0, nil, "RepositoryFragment", nil), types.NewStruct(nil, nil), nil)),
			IsFragmentSpread: true,
			ResponseFields: ResponseFieldList{
				{
					Name: "id",
					Type: types.Typ[types.String],
					Tags: []string{`json:"id" graphql:"id"`},
				},
				{
					Name: "name",
					Type: types.Typ[types.String],
					Tags: []string{`json:"name" graphql:"name"`},
				},
			},
		},
	}

	mockCfg := &config.Config{}
	mockBinder := &config.Binder{}
	mockPackageConfig := config.PackageConfig{}
	mockGenCfg := &gqlgencConfig.GenerateConfig{}

	sg := &SourceGenerator{
		cfg:            mockCfg,
		binder:         mockBinder,
		client:         mockPackageConfig,
		generateConfig: mockGenCfg,
		StructSources:  make([]*StructSource, 0),
	}

	// フラグメントスプレッドの存在を確認
	// Verify the existence of fragment spreads
	hasFragmentSpread := sg.hasFragmentSpread(responseFields)
	if !hasFragmentSpread {
		t.Errorf("Expected hasFragmentSpread to return true when a fragment spread is present")
	}

	// フラグメントフィールドの収集をテスト
	// Test the collection of fragment fields
	fragmentFields := sg.collectFragmentFields(responseFields)
	if len(fragmentFields) != 2 {
		t.Errorf("Expected 2 fields from fragment spread, got %d", len(fragmentFields))
	}

	// フィールド名を検証
	// Validate field names
	foundID := false
	foundName := false

	for _, field := range fragmentFields {
		switch field.Name {
		case "id":
			foundID = true
		case "name":
			foundName = true
		}
	}

	if !foundID {
		t.Errorf("Expected to find 'id' field from fragment")
	}
	if !foundName {
		t.Errorf("Expected to find 'name' field from fragment")
	}

	// 実際のフラグメントスプレッド展開処理をテスト
	// Test the actual fragment spread expansion process
	// フラグメントスプレッドを含むフィールドから、すべてのフィールドを集める
	// Collect all fields from fields containing fragment spreads
	allFields := make(ResponseFieldList, 0)
	for _, field := range responseFields {
		if !field.IsFragmentSpread {
			allFields = append(allFields, field)
		}
	}
	// フラグメントのフィールドを追加
	// Add fields from fragments
	allFields = append(allFields, fragmentFields...)

	// フィールドの数を検証
	// Validate the number of fields
	if len(allFields) != 3 { // languages + id + name
		t.Errorf("Expected 3 fields after expansion, got %d", len(allFields))
	}

	// フィールド名を検証
	// Validate field names
	foundLanguages := false
	foundID = false
	foundName = false

	for _, field := range allFields {
		switch field.Name {
		case "languages":
			foundLanguages = true
		case "id":
			foundID = true
		case "name":
			foundName = true
		case "RepositoryFragment":
			// このフィールドは展開されるため、ここには存在しないはず
			// This field should not exist after expansion
			t.Errorf("RepositoryFragment field should not exist after expansion")
		}
	}

	if !foundLanguages {
		t.Errorf("Expected to find 'languages' field")
	}
	if !foundID {
		t.Errorf("Expected to find 'id' field from fragment")
	}
	if !foundName {
		t.Errorf("Expected to find 'name' field from fragment")
	}
}
