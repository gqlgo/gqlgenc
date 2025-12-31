package clientgenv2

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"
	gqlgencConfig "github.com/gqlgo/gqlgenc/config"
	"github.com/vektah/gqlparser/v2/ast"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Argument struct {
	Variable string
	Type     types.Type
}

type ResponseField struct {
	Name             string
	IsFragmentSpread bool
	IsInlineFragment bool
	Type             types.Type
	Tags             []string
	ResponseFields   ResponseFieldList
}

func (r ResponseField) FieldTypeString() string {
	fullFieldType := r.Type.String()
	parts := strings.Split(fullFieldType, ".")
	return parts[len(parts)-1]
}

type ResponseFieldList []*ResponseField

func (rs ResponseFieldList) IsFragmentSpread() bool {
	if len(rs) != 1 {
		return false
	}

	return rs[0].IsFragmentSpread
}

func (rs ResponseFieldList) StructType() *types.Struct {
	vars := make([]*types.Var, 0)
	structTags := make([]string, 0)
	for _, field := range rs {
		vars = append(vars, types.NewVar(0, nil, templates.ToGo(field.Name), field.Type))
		structTags = append(structTags, strings.Join(field.Tags, " "))
	}
	return types.NewStruct(vars, structTags)
}

func (rs ResponseFieldList) IsFragment() bool {
	if len(rs) != 1 {
		return false
	}

	return rs[0].IsInlineFragment || rs[0].IsFragmentSpread
}

func (rs ResponseFieldList) IsBasicType() bool {
	return len(rs) == 0
}

func (rs ResponseFieldList) IsStructType() bool {
	return len(rs) > 0 && !rs.IsFragment()
}

func (rs ResponseFieldList) MapByName() map[string]*ResponseField {
	res := make(map[string]*ResponseField)
	for _, field := range rs {
		res[field.Name] = field
	}
	return res
}

func (rs ResponseFieldList) SortByName() ResponseFieldList {
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Name < rs[j].Name
	})
	return rs
}

type StructGenerator struct {
	// Create fields based on this ResponseFieldList
	currentResponseFieldList ResponseFieldList
	// Struct sources that will no longer be created due to merging
	preMergedStructSources []*StructSource
	// Struct sources that will be created due to merging
	postMergedStructSources []*StructSource
}

func NewStructGenerator(responseFieldList ResponseFieldList) *StructGenerator {
	currentFields := make(ResponseFieldList, 0)
	fragmentChildrenFields := make(ResponseFieldList, 0)
	for _, field := range responseFieldList {
		if field.IsFragmentSpread {
			fragmentChildrenFields = append(fragmentChildrenFields, field.ResponseFields...)
		} else {
			currentFields = append(currentFields, field)
		}
	}

	preMergedStructSources := make([]*StructSource, 0)

	for _, field := range responseFieldList {
		if field.IsFragmentSpread {
			preMergedStructSources = append(preMergedStructSources, &StructSource{
				Name: field.FieldTypeString(),
				Type: field.ResponseFields.StructType(),
			})
		}
	}

	currentFields, preMergedStructSources, postMergedStructSources := mergeFieldsRecursively(currentFields, fragmentChildrenFields, preMergedStructSources, nil)
	return &StructGenerator{
		currentResponseFieldList: currentFields,
		preMergedStructSources:   preMergedStructSources,
		postMergedStructSources:  postMergedStructSources,
	}
}

func mergeFieldsRecursively(targetFields, sourceFields ResponseFieldList, preMerged, postMerged []*StructSource) (ResponseFieldList, []*StructSource, []*StructSource) {
	responseFieldList := make(ResponseFieldList, 0)
	targetFieldsMap := targetFields.MapByName()
	newPreMerged := preMerged
	newPostMerged := postMerged

	for _, sourceField := range sourceFields {
		if targetField, ok := targetFieldsMap[sourceField.Name]; ok {
			if targetField.ResponseFields.IsBasicType() {
				continue
			}
			newPreMerged = append(newPreMerged, &StructSource{
				Name: sourceField.FieldTypeString(),
				Type: sourceField.ResponseFields.StructType(),
			})
			newPreMerged = append(newPreMerged, &StructSource{
				Name: targetField.FieldTypeString(),
				Type: targetField.ResponseFields.StructType(),
			})

			targetField.ResponseFields, newPreMerged, newPostMerged = mergeFieldsRecursively(targetField.ResponseFields, sourceField.ResponseFields, newPreMerged, newPostMerged)

			newPostMerged = append(newPostMerged, &StructSource{
				Name: targetField.FieldTypeString(),
				Type: targetField.ResponseFields.StructType(),
			})
		} else {
			targetFieldsMap[sourceField.Name] = sourceField
		}
	}
	for _, field := range targetFieldsMap {
		responseFieldList = append(responseFieldList, field)
	}
	responseFieldList = responseFieldList.SortByName()
	return responseFieldList, newPreMerged, newPostMerged
}

func structSourcesMapByTypeName(sources []*StructSource) map[string]*StructSource {
	res := make(map[string]*StructSource)
	for _, source := range sources {
		res[source.Name] = source
	}
	return res
}

func (g *StructGenerator) MergedStructSources(sources []*StructSource) []*StructSource {
	preMergedStructSourcesMap := structSourcesMapByTypeName(g.preMergedStructSources)
	res := make([]*StructSource, 0)
	// remove pre-merged struct
	for _, source := range sources {
		// when name is same, remove it
		if _, ok := preMergedStructSourcesMap[source.Name]; ok {
			continue
		}
		res = append(res, source)
	}

	// append post-merged struct
	res = append(res, g.postMergedStructSources...)

	return res
}

func (g *StructGenerator) GetCurrentResponseFieldList() ResponseFieldList {
	return g.currentResponseFieldList
}

type StructSource struct {
	Name string
	Type types.Type
}

type SourceGenerator struct {
	cfg            *config.Config
	binder         *config.Binder
	client         config.PackageConfig
	generateConfig *gqlgencConfig.GenerateConfig
	StructSources  []*StructSource
}

func NewSourceGenerator(cfg *config.Config, client config.PackageConfig, generateConfig *gqlgencConfig.GenerateConfig) *SourceGenerator {
	return &SourceGenerator{
		cfg:            cfg,
		binder:         cfg.NewBinder(),
		client:         client,
		generateConfig: generateConfig,
		StructSources:  []*StructSource{},
	}
}

func (r *SourceGenerator) NewResponseFields(selectionSet ast.SelectionSet, typeName string) ResponseFieldList {
	responseFields := make(ResponseFieldList, 0, len(selectionSet))
	for _, selection := range selectionSet {
		responseFields = append(responseFields, r.NewResponseField(selection, typeName))
	}

	return responseFields
}

func NewLayerTypeName(base, thisField string) string {
	return fmt.Sprintf("%s_%s", cases.Title(language.Und, cases.NoLower).String(base), thisField)
}

func (r *SourceGenerator) NewResponseField(selection ast.Selection, typeName string) *ResponseField {
	var isOptional bool
	switch selection := selection.(type) {
	case *ast.Field:
		typeName = NewLayerTypeName(typeName, templates.ToGo(selection.Alias))
		fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, typeName)

		isOptional = !selection.Definition.Type.NonNull

		var baseType types.Type
		switch {
		case fieldsResponseFields.IsBasicType():
			baseType = r.Type(selection.Definition.Type.Name())
		case fieldsResponseFields.IsFragment():
			// 子フィールドがFragmentの場合はこのFragmentがフィールドの型になる
			// if a child field is fragment, this field type became fragment.
			baseType = fieldsResponseFields[0].Type
		case fieldsResponseFields.IsStructType():
			// 子フィールドにFragmentがある場合は、現在のフィールドとマージする
			// if there is a fragment in child fields, merge it with the current field
			generator := NewStructGenerator(fieldsResponseFields)

			// restruct struct sources
			r.StructSources = generator.MergedStructSources(r.StructSources)

			// append current struct
			structType := generator.GetCurrentResponseFieldList().StructType()
			r.StructSources = append(r.StructSources, &StructSource{
				Name: typeName,
				Type: structType,
			})
			baseType = types.NewNamed(
				types.NewTypeName(0, r.client.Pkg(), typeName, nil),
				structType,
				nil,
			)
		default:
			// ここにきたらバグ
			// here is bug
			panic("not match type")
		}

		// GraphQLの定義がオプショナルのはtypeのポインタ型が返り、配列の定義場合はポインタのスライスの型になって返ってきます
		// return pointer type then optional type or slice pointer then slice type of definition in GraphQL.
		typ := r.binder.CopyModifiersFromAst(selection.Definition.Type, baseType)

		// json tag
		jsonTag := fmt.Sprintf(`json:"%s`, selection.Alias)
		if isOptional {
			if r.generateConfig.EnableClientJsonOmitemptyTag != nil && *r.generateConfig.EnableClientJsonOmitemptyTag {
				jsonTag += `,omitempty`
			}
			if r.generateConfig.EnableClientJsonOmitzeroTag != nil && *r.generateConfig.EnableClientJsonOmitzeroTag {
				jsonTag += `,omitzero`
			}
		}
		jsonTag += `"`

		// graphql tag
		tags := []string{
			jsonTag,
			fmt.Sprintf(`graphql:"%s"`, selection.Alias),
		}

		return &ResponseField{
			Name:           selection.Alias,
			Type:           typ,
			Tags:           tags,
			ResponseFields: fieldsResponseFields,
		}

	case *ast.FragmentSpread:
		// この構造体はテンプレート側で使われることはなく、ast.FieldでFragment判定するために使用する
		fieldsResponseFields := r.NewResponseFields(selection.Definition.SelectionSet, NewLayerTypeName(typeName, templates.ToGo(selection.Name)))
		baseType := types.NewNamed(
			types.NewTypeName(0, r.client.Pkg(), templates.ToGo(selection.Name), nil),
			fieldsResponseFields.StructType(),
			nil,
		)

		var typ types.Type = baseType
		if r.cfg.StructFieldsAlwaysPointers {
			typ = types.NewPointer(baseType)
		}

		return &ResponseField{
			Name:             selection.Name,
			Type:             typ,
			IsFragmentSpread: true,
			ResponseFields:   fieldsResponseFields,
		}

	case *ast.InlineFragment:
		// InlineFragmentは子要素をそのままstructとしてもつので、ここで、構造体の型を作成します
		// InlineFragment has child elements, so create a struct type here
		name := NewLayerTypeName(typeName, templates.ToGo(selection.TypeCondition))
		fieldsResponseFields := r.NewResponseFields(selection.SelectionSet, name)

		// if single fields that is also a fragment spread, reuse that fragment
		if len(fieldsResponseFields) == 1 && fieldsResponseFields[0].IsFragmentSpread {
			typ := types.NewNamed(
				types.NewTypeName(0, r.client.Pkg(), templates.ToGo(fieldsResponseFields[0].Name), nil),
				fieldsResponseFields.StructType(),
				nil,
			)

			return &ResponseField{
				Name:           selection.TypeCondition,
				Type:           typ,
				Tags:           []string{fmt.Sprintf(`graphql:"... on %s"`, selection.TypeCondition)},
				ResponseFields: fieldsResponseFields,
			}
		}

		hasFragmentSpread := r.hasFragmentSpread(fieldsResponseFields)
		fragmentFields := r.collectFragmentFields(fieldsResponseFields)

		// フラグメントスプレッドがある場合
		// if there is a fragment spread
		if hasFragmentSpread {
			// フラグメントからの全フィールドを集めます
			// collect all fields from fragment
			allFields := make(ResponseFieldList, 0)
			for _, field := range fieldsResponseFields {
				if !field.IsFragmentSpread {
					allFields = append(allFields, field)
				}
			}
			// フラグメントのフィールドを追加
			// append fragment fields
			allFields = append(allFields, fragmentFields...)

			// 構造体を生成
			// generate struct
			structType := allFields.StructType()
			r.StructSources = append(r.StructSources, &StructSource{
				Name: name,
				Type: structType,
			})
			typ := types.NewNamed(
				types.NewTypeName(0, r.client.Pkg(), name, nil),
				structType,
				nil,
			)

			return &ResponseField{
				Name:             selection.TypeCondition,
				Type:             typ,
				IsInlineFragment: true,
				Tags:             []string{fmt.Sprintf(`graphql:"... on %s"`, selection.TypeCondition)},
				ResponseFields:   allFields.SortByName(),
			}
		}
		// フラグメントスプレッドがない場合
		// if there is no fragment spread
		structType := fieldsResponseFields.StructType()
		r.StructSources = append(r.StructSources, &StructSource{
			Name: name,
			Type: structType,
		})
		typ := types.NewNamed(
			types.NewTypeName(0, r.client.Pkg(), name, nil),
			structType,
			nil,
		)

		return &ResponseField{
			Name:             selection.TypeCondition,
			Type:             typ,
			IsInlineFragment: true,
			Tags:             []string{fmt.Sprintf(`graphql:"... on %s"`, selection.TypeCondition)},
			ResponseFields:   fieldsResponseFields.SortByName(),
		}
	}

	panic("unexpected selection type")
}

func (r *SourceGenerator) OperationArguments(variableDefinitions ast.VariableDefinitionList) []*Argument {
	argumentTypes := make([]*Argument, 0, len(variableDefinitions))
	for _, v := range variableDefinitions {
		argumentTypes = append(argumentTypes, &Argument{
			Variable: v.Variable,
			Type:     r.binder.CopyModifiersFromAst(v.Type, r.Type(v.Type.Name())),
		})
	}

	return argumentTypes
}

// Typeの引数に渡すtypeNameは解析した結果からselectionなどから求めた型の名前を渡さなければいけない
func (r *SourceGenerator) Type(typeName string) types.Type {
	goType, err := r.binder.FindTypeFromName(r.cfg.Models[typeName].Model[0])
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	return goType
}

func (r *SourceGenerator) expandFragmentFields(responseFields ResponseFieldList) ResponseFieldList {
	result := make(ResponseFieldList, 0, len(responseFields))
	for _, field := range responseFields {
		if field.IsFragmentSpread {
			for _, fragmentField := range field.ResponseFields {
				result = append(result, fragmentField)
			}
		} else {
			result = append(result, field)
		}
	}
	return result
}

func (r *SourceGenerator) hasFragmentSpread(fields ResponseFieldList) bool {
	for _, field := range fields {
		if field.IsFragmentSpread {
			return true
		}
	}
	return false
}

func (r *SourceGenerator) collectFragmentFields(fields ResponseFieldList) ResponseFieldList {
	var fragmentFields ResponseFieldList
	for _, field := range fields {
		if field.IsFragmentSpread {
			fragmentFields = append(fragmentFields, field.ResponseFields...)
		}
	}
	return fragmentFields
}
